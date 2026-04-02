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

// Package auth provides OIDC authentication endpoints for the platform.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	oidc "github.com/coreos/go-oidc"
	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
)

// OIDCConfig holds OIDC provider settings.
type OIDCConfig struct {
	IssuerURL string
	ClientID  string
	// RedirectURL is built from HubExternalURL + "/auth/callback".
	RedirectURL string
	Scopes      []string
}

// authState is encoded into the OAuth2 state parameter.
type authState struct {
	RedirectURL  string `json:"redirectURL"`
	SessionID    string `json:"sessionID"`
	CodeVerifier string `json:"codeVerifier"`
}

// CallbackResponse is returned to the browser/CLI after successful OIDC login.
type CallbackResponse struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt"`
	Email        string `json:"email"`
	ClusterName  string `json:"clusterName,omitempty"`
	// OIDC config needed by the CLI to refresh tokens.
	IssuerURL string `json:"issuerURL,omitempty"`
	ClientID  string `json:"clientID,omitempty"`
	// Hub URL so the CLI can build the kubeconfig server URL.
	HubURL string `json:"hubURL,omitempty"`
}

// WorkspaceProvisioner creates tenant workspaces and RBAC on login.
// Implemented by the kcp bootstrapper.
type WorkspaceProvisioner interface {
	// EnsureTenantWorkspace creates the workspace and RBAC if they don't exist.
	// Returns the full kcp workspace path (e.g. "root:platform:tenants:u-abc123").
	EnsureTenantWorkspace(ctx context.Context, workspaceName, oidcUserName string) (string, error)
}

// Handler provides OAuth2/OIDC authentication endpoints.
type Handler struct {
	oidcProvider   *oidc.Provider
	oauth2Config   *oauth2.Config
	oidcConfig     *OIDCConfig
	hubExternalURL string
	provisioner    WorkspaceProvisioner
	devMode        bool
	logger         klog.Logger
}

// NewHandler creates a new OIDC auth handler.
// provisioner may be nil if workspace auto-creation is not needed.
func NewHandler(ctx context.Context, config *OIDCConfig, hubExternalURL string, provisioner WorkspaceProvisioner, devMode bool) (*Handler, error) {
	if config.IssuerURL == "" {
		return nil, fmt.Errorf("OIDC issuer URL is required")
	}

	providerCtx := ctx
	if devMode {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev mode only
		}
		httpClient := &http.Client{Transport: tr}
		providerCtx = oidc.ClientContext(ctx, httpClient)
	}

	provider, err := oidc.NewProvider(providerCtx, config.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("creating OIDC provider: %w", err)
	}

	if config.Scopes == nil {
		config.Scopes = []string{oidc.ScopeOpenID, "profile", "email", "offline_access"}
	}
	if config.RedirectURL == "" {
		config.RedirectURL = hubExternalURL + "/auth/callback"
	}

	oauth2Config := &oauth2.Config{
		ClientID:    config.ClientID,
		RedirectURL: config.RedirectURL,
		Endpoint:    provider.Endpoint(),
		Scopes:      config.Scopes,
	}

	return &Handler{
		oidcProvider:   provider,
		oauth2Config:   oauth2Config,
		oidcConfig:     config,
		hubExternalURL: hubExternalURL,
		provisioner:    provisioner,
		devMode:        devMode,
		logger:         klog.Background().WithName("auth-handler"),
	}, nil
}

// Verifier returns the OIDC token verifier for use by other components.
func (h *Handler) Verifier() *oidc.IDTokenVerifier {
	return h.oidcProvider.Verifier(&oidc.Config{ClientID: h.oidcConfig.ClientID})
}

// RegisterRoutes registers auth routes on the given router.
func (h *Handler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/auth/authorize", h.HandleAuthorize).Methods("GET")
	router.HandleFunc("/auth/callback", h.HandleCallback).Methods("GET")
}

// HandleAuthorize redirects to the OIDC provider for authentication.
//
// Browser/portal flow:
//
//	GET /auth/authorize?redirect_uri=<console_callback_url>
//
// The handler generates a PKCE code_verifier, stores it in the OAuth2 state,
// and redirects to the OIDC provider with the S256 code_challenge.
func (h *Handler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		http.Error(w, "missing redirect_uri parameter", http.StatusBadRequest)
		return
	}

	if err := h.validateRedirectURI(redirectURI); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Generate PKCE code_verifier server-side.
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		http.Error(w, "failed to generate PKCE verifier", http.StatusInternalServerError)
		return
	}

	sessionID := r.URL.Query().Get("s")
	if sessionID == "" {
		sessionID = "browser"
	}

	state := authState{
		RedirectURL:  redirectURI,
		SessionID:    sessionID,
		CodeVerifier: codeVerifier,
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "failed to encode state", http.StatusInternalServerError)
		return
	}
	stateParam := base64.URLEncoding.EncodeToString(stateJSON)

	authURL := h.oauth2Config.AuthCodeURL(stateParam, oauth2.S256ChallengeOption(codeVerifier))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// oidcClaims captures all relevant claims from the ID token.
// Different OIDC providers use different claim names.
type oidcClaims struct {
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Sub               string `json:"sub"`
}

// username returns the best available username from the claims.
func (c *oidcClaims) username() string {
	if c.Email != "" {
		return c.Email
	}
	if c.PreferredUsername != "" {
		return c.PreferredUsername
	}
	if c.Name != "" {
		return c.Name
	}
	return c.Sub
}

// HandleCallback handles the OIDC callback after authentication.
func (h *Handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	code := r.URL.Query().Get("code")
	stateParam := r.URL.Query().Get("state")
	if code == "" || stateParam == "" {
		http.Error(w, "missing code or state parameter", http.StatusBadRequest)
		return
	}

	stateJSON, err := base64.URLEncoding.DecodeString(stateParam)
	if err != nil {
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}
	var state authState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		http.Error(w, "invalid state payload", http.StatusBadRequest)
		return
	}

	// Exchange code for tokens using PKCE verifier (no client secret).
	exchangeCtx := ctx
	if h.devMode {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev mode only
		}
		httpClient := &http.Client{Transport: tr}
		exchangeCtx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}

	token, err := h.oauth2Config.Exchange(exchangeCtx, code, oauth2.VerifierOption(state.CodeVerifier))
	if err != nil {
		h.logger.Error(err, "failed to exchange code for token")
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing id_token in token response", http.StatusInternalServerError)
		return
	}

	// Verify the ID token.
	verifier := h.oidcProvider.Verifier(&oidc.Config{ClientID: h.oidcConfig.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		h.logger.Error(err, "failed to verify ID token")
		http.Error(w, "token verification failed", http.StatusInternalServerError)
		return
	}

	// Parse claims — try multiple claim names for compatibility.
	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		h.logger.Error(err, "failed to parse ID token claims")
		http.Error(w, "failed to parse claims", http.StatusInternalServerError)
		return
	}

	// Log all claims for debugging.
	var allClaims map[string]interface{}
	_ = idToken.Claims(&allClaims)
	h.logger.Info("OIDC login", "sub", claims.Sub, "email", claims.Email,
		"preferred_username", claims.PreferredUsername, "name", claims.Name,
		"allClaims", allClaims)

	username := claims.username()
	if username == "" {
		h.logger.Error(nil, "no usable identity claim found in token", "claims", allClaims)
		http.Error(w, "no usable identity in token (missing email, preferred_username, name, sub)", http.StatusInternalServerError)
		return
	}

	// Derive workspace name from the user identity.
	wsName := workspaceNameForUser(username)

	// The OIDC user name as kcp sees it (with oidc: prefix from RootShard config).
	oidcUserName := "oidc:" + username

	// Provision workspace and RBAC if a provisioner is configured.
	clusterPath := ""
	if h.provisioner != nil {
		var err error
		clusterPath, err = h.provisioner.EnsureTenantWorkspace(ctx, wsName, oidcUserName)
		if err != nil {
			h.logger.Error(err, "failed to provision tenant workspace", "workspace", wsName, "user", oidcUserName)
			http.Error(w, "failed to provision workspace", http.StatusInternalServerError)
			return
		}
		h.logger.Info("Provisioned tenant workspace", "workspace", wsName, "clusterPath", clusterPath, "user", oidcUserName)
	}

	resp := CallbackResponse{
		IDToken:      rawIDToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry.Unix(),
		Email:        username,
		ClusterName:  clusterPath,
		IssuerURL:    h.oidcConfig.IssuerURL,
		ClientID:     h.oidcConfig.ClientID,
		HubURL:       h.hubExternalURL,
	}
	respJSON, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	// Redirect back to the console/CLI with the response as a query parameter.
	encoded := base64.URLEncoding.EncodeToString(respJSON)
	redirectURL := state.RedirectURL + "?response=" + encoded
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// validateRedirectURI checks that the redirect URI is safe.
func (h *Handler) validateRedirectURI(redirectURI string) error {
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		return fmt.Errorf("invalid redirect_uri: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("redirect_uri must be an absolute URL")
	}

	host := strings.Split(parsed.Host, ":")[0]
	if host == "localhost" || host == "127.0.0.1" {
		return nil
	}

	hubParsed, err := url.Parse(h.hubExternalURL)
	if err != nil {
		return fmt.Errorf("invalid hub external URL configuration")
	}
	hubHost := strings.Split(hubParsed.Host, ":")[0]
	if host != hubHost {
		return fmt.Errorf("redirect_uri origin must match hub external URL")
	}

	return nil
}

// workspaceNameForUser derives a DNS-safe workspace name from a user identity.
func workspaceNameForUser(username string) string {
	h := sha256.Sum256([]byte(username))
	return "u-" + hex.EncodeToString(h[:])[:10]
}

// generateCodeVerifier creates a cryptographically random PKCE code_verifier.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
