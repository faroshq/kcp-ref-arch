/*
Copyright 2026 The KCP Reference Architecture Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package proxy reverse-proxies authenticated requests to kcp.
package proxy

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	oidc "github.com/coreos/go-oidc"
	"golang.org/x/oauth2"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"github.com/faroshq/kcp-ref-arch/project/platform/apis/auth"
)

// KCPProxy is a reverse proxy that authenticates requests via OIDC or static
// tokens and forwards them to kcp.
type KCPProxy struct {
	kcpTarget        *url.URL
	transport        http.RoundTripper // admin credentials transport
	passTransport    http.RoundTripper // TLS-only, no credentials
	verifier         *oidc.IDTokenVerifier
	verifyCtx        context.Context
	staticAuthTokens []string
	hubExternalURL   string
	devMode          bool
	logger           klog.Logger
}

// New creates a reverse proxy to kcp.
// verifier may be nil when only static token auth is used.
func New(kcpConfig *rest.Config, verifier *oidc.IDTokenVerifier, staticAuthTokens []string, hubExternalURL string, devMode bool) (*KCPProxy, error) {
	target, err := url.Parse(kcpConfig.Host)
	if err != nil {
		return nil, err
	}

	transportConfig := rest.CopyConfig(kcpConfig)
	if devMode {
		if len(transportConfig.CAData) == 0 && transportConfig.CAFile == "" {
			transportConfig.Insecure = true
		}
	}
	transport, err := rest.TransportFor(transportConfig)
	if err != nil {
		return nil, fmt.Errorf("building kcp transport: %w", err)
	}

	// Passthrough transport: TLS only, no admin credentials.
	passConfig := &rest.Config{
		Host: kcpConfig.Host,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: transportConfig.Insecure,
			CAData:   transportConfig.CAData,
			CAFile:   transportConfig.CAFile,
		},
	}
	passTransport, err := rest.TransportFor(passConfig)
	if err != nil {
		return nil, fmt.Errorf("building passthrough transport: %w", err)
	}

	// Build a context for OIDC key fetches (needs insecure client in dev mode).
	verifyCtx := context.Background()
	if devMode {
		insecureClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev mode only
			},
		}
		verifyCtx = context.WithValue(verifyCtx, oauth2.HTTPClient, insecureClient)
	}

	return &KCPProxy{
		kcpTarget:        target,
		transport:        transport,
		passTransport:    passTransport,
		verifier:         verifier,
		verifyCtx:        verifyCtx,
		staticAuthTokens: staticAuthTokens,
		hubExternalURL:   hubExternalURL,
		devMode:          devMode,
		logger:           klog.Background().WithName("kcp-proxy"),
	}, nil
}

// ServeHTTP validates the bearer token and proxies the request to kcp.
// Authentication is attempted in order: static tokens, OIDC ID tokens.
func (p *KCPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token == "" {
		writeUnauthorized(w)
		return
	}

	// 1. Check static tokens.
	for _, staticToken := range p.staticAuthTokens {
		if staticToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(staticToken)) == 1 {
			p.serveStaticToken(w, r, token)
			return
		}
	}

	// 2. Try OIDC verification.
	if p.verifier != nil {
		idToken, err := p.verifier.Verify(p.verifyCtx, token)
		if err == nil {
			p.serveOIDC(w, r, idToken)
			return
		}
	}

	writeUnauthorized(w)
}

// serveOIDC handles OIDC-authenticated requests by proxying to kcp with
// admin credentials. The OIDC user identity is passed via Impersonate-User
// header so kcp applies RBAC for the actual user.
func (p *KCPProxy) serveOIDC(w http.ResponseWriter, r *http.Request, idToken *oidc.IDToken) {
	var claims struct {
		Email             string   `json:"email"`
		PreferredUsername string   `json:"preferred_username"`
		Name              string   `json:"name"`
		Sub               string   `json:"sub"`
		Groups            []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		p.logger.Error(err, "failed to parse ID token claims")
		writeError(w, http.StatusInternalServerError, "failed to parse token claims")
		return
	}

	// Determine the best username from available claims.
	username := claims.Email
	if username == "" {
		username = claims.PreferredUsername
	}
	if username == "" {
		username = claims.Name
	}
	if username == "" {
		username = claims.Sub
	}
	if username == "" {
		p.logger.Error(nil, "no usable identity claim in token")
		writeError(w, http.StatusUnauthorized, "no usable identity in token")
		return
	}

	// kcp OIDC username prefix is "oidc:" (configured in RootShard).
	// We impersonate as the OIDC user so kcp applies their RBAC rules.
	oidcUser := "oidc:" + username

	target := *p.kcpTarget
	logger := p.logger

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			// Scope bare /api paths to /clusters/root.
			if !strings.HasPrefix(req.URL.Path, "/clusters/") {
				req.URL.Path = "/clusters/root" + req.URL.Path
			}

			// Remove user's token, use admin transport credentials.
			req.Header.Del("Authorization")

			// Impersonate the OIDC user so kcp enforces their RBAC.
			req.Header.Set("Impersonate-User", oidcUser)
			req.Header.Del("Impersonate-Group")
			for _, group := range claims.Groups {
				req.Header.Add("Impersonate-Group", "oidc:"+group)
			}
		},
		Transport: p.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error(err, "proxy upstream error (OIDC)", "method", r.Method, "path", r.URL.Path)
			writeError(w, http.StatusBadGateway, "upstream error")
		},
	}

	proxy.ServeHTTP(w, r)
}

// serveStaticToken proxies the request to kcp with the caller's static token.
func (p *KCPProxy) serveStaticToken(w http.ResponseWriter, r *http.Request, token string) {
	target := *p.kcpTarget
	logger := p.logger

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			if !strings.HasPrefix(req.URL.Path, "/clusters/") {
				req.URL.Path = "/clusters/root" + req.URL.Path
			}

			req.Header.Set("Authorization", "Bearer "+token)
		},
		Transport: p.passTransport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error(err, "proxy upstream error", "method", r.Method, "path", r.URL.Path)
			writeError(w, http.StatusBadGateway, "upstream error")
		},
	}

	proxy.ServeHTTP(w, r)
}

// HandleTokenLogin handles static token login requests.
func (p *KCPProxy) HandleTokenLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	token := extractToken(r)
	if token == "" {
		writeUnauthorized(w)
		return
	}

	validToken := false
	for _, staticToken := range p.staticAuthTokens {
		if staticToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(staticToken)) == 1 {
			validToken = true
			break
		}
	}
	if !validToken {
		writeUnauthorized(w)
		return
	}

	tokenHash := sha256.Sum256([]byte("static-token/" + token))
	subHash := hex.EncodeToString(tokenHash[:])[:63]
	userID := fmt.Sprintf("platform:static:%s", subHash[:16])

	kubeconfigBytes, err := p.generateKubeconfig(token)
	if err != nil {
		p.logger.Error(err, "failed to generate kubeconfig")
		writeError(w, http.StatusInternalServerError, "failed to generate kubeconfig")
		return
	}

	resp := auth.LoginResponse{
		Kubeconfig: kubeconfigBytes,
		Email:      userID + "@platform.local",
		UserID:     userID,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		p.logger.Error(err, "failed to encode login response")
	}
}

func (p *KCPProxy) generateKubeconfig(token string) ([]byte, error) {
	config := clientcmdapi.NewConfig()

	config.Clusters["platform"] = &clientcmdapi.Cluster{
		Server:                p.hubExternalURL,
		InsecureSkipTLSVerify: p.devMode,
	}

	config.AuthInfos["platform"] = &clientcmdapi.AuthInfo{
		Token: token,
	}

	config.Contexts["platform"] = &clientcmdapi.Context{
		Cluster:  "platform",
		AuthInfo: "platform",
	}

	config.CurrentContext = "platform"

	return clientcmd.Write(*config)
}

func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

func writeUnauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "Unauthorized")
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":%q,"code":%d}`, message, code)
}
