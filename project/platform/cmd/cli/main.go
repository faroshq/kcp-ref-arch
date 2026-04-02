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

package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	oidc "github.com/coreos/go-oidc"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"github.com/faroshq/kcp-ref-arch/project/platform/apis/auth"
	platformauth "github.com/faroshq/kcp-ref-arch/project/platform/pkg/auth"
)

func main() {
	cmd := &cobra.Command{
		Use:   "platform-cli",
		Short: "Platform CLI - authenticate and interact with the platform",
	}

	cmd.AddCommand(newLoginCommand())
	cmd.AddCommand(newGetTokenCommand())

	goFlags := flag.NewFlagSet("", flag.ContinueOnError)
	klog.InitFlags(goFlags)
	cmd.PersistentFlags().AddGoFlagSet(goFlags)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := cmd.ExecuteContext(ctx); err != nil {
		klog.Fatal(err)
		os.Exit(1)
	}
}

// =============================================================================
// Token cache
// =============================================================================

// tokenCache is persisted to ~/.config/platform/tokens/<hash>.json.
type tokenCache struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt"`
	IssuerURL    string `json:"issuerUrl"`
	ClientID     string `json:"clientId"`
}

func tokenCacheDir() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfgDir, "platform", "tokens")
}

func tokenCacheKey(issuerURL, clientID string) string {
	h := sha256.Sum256([]byte(issuerURL + "\n" + clientID))
	return hex.EncodeToString(h[:])[:32]
}

func tokenCachePath(issuerURL, clientID string) string {
	return filepath.Join(tokenCacheDir(), tokenCacheKey(issuerURL, clientID)+".json")
}

func loadTokenCache(issuerURL, clientID string) (*tokenCache, error) {
	data, err := os.ReadFile(tokenCachePath(issuerURL, clientID))
	if err != nil {
		return nil, err
	}
	var tc tokenCache
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}

func saveTokenCache(tc *tokenCache) error {
	dir := tokenCacheDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating token cache dir: %w", err)
	}
	data, err := json.MarshalIndent(tc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokenCachePath(tc.IssuerURL, tc.ClientID), data, 0600)
}

func (tc *tokenCache) isExpired() bool {
	return time.Now().Unix() > tc.ExpiresAt-30
}

// =============================================================================
// get-token command (exec credential plugin)
// =============================================================================

func newGetTokenCommand() *cobra.Command {
	var (
		issuerURL             string
		clientID              string
		insecureSkipTLSVerify bool
	)

	cmd := &cobra.Command{
		Use:    "get-token",
		Short:  "Get an OIDC token (exec credential plugin for kubectl)",
		Hidden: true, // Called by kubectl, not directly by users.
		RunE: func(cmd *cobra.Command, args []string) error {
			if issuerURL == "" || clientID == "" {
				return fmt.Errorf("--oidc-issuer-url and --oidc-client-id are required")
			}
			return runGetToken(cmd.Context(), issuerURL, clientID, insecureSkipTLSVerify)
		},
	}

	cmd.Flags().StringVar(&issuerURL, "oidc-issuer-url", "", "OIDC provider issuer URL")
	cmd.Flags().StringVar(&clientID, "oidc-client-id", "", "OIDC client ID")
	cmd.Flags().BoolVar(&insecureSkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS verification for OIDC provider")

	return cmd
}

func runGetToken(ctx context.Context, issuerURL, clientID string, insecure bool) error {
	tc, err := loadTokenCache(issuerURL, clientID)
	if err != nil {
		return fmt.Errorf("no cached token found — run 'platform-cli login' first: %w", err)
	}

	// Refresh if expired.
	if tc.isExpired() {
		if tc.RefreshToken == "" {
			return fmt.Errorf("token expired and no refresh token — run 'platform-cli login' again")
		}
		if err := refreshToken(ctx, tc, insecure); err != nil {
			return fmt.Errorf("token refresh failed — run 'platform-cli login' again: %w", err)
		}
		if err := saveTokenCache(tc); err != nil {
			klog.V(2).Infof("Warning: failed to save refreshed token: %v", err)
		}
	}

	// Output ExecCredential JSON to stdout.
	expiry := time.Unix(tc.ExpiresAt, 0).UTC().Format(time.RFC3339)
	cred := map[string]interface{}{
		"apiVersion": "client.authentication.k8s.io/v1beta1",
		"kind":       "ExecCredential",
		"status": map[string]interface{}{
			"token":               tc.IDToken,
			"expirationTimestamp": expiry,
		},
	}

	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("encoding ExecCredential: %w", err)
	}
	_, err = os.Stdout.Write(data)
	return err
}

func refreshToken(ctx context.Context, tc *tokenCache, insecure bool) error {
	providerCtx := ctx
	if insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
		httpClient := &http.Client{Transport: tr}
		providerCtx = oidc.ClientContext(ctx, httpClient)
	}

	provider, err := oidc.NewProvider(providerCtx, tc.IssuerURL)
	if err != nil {
		return fmt.Errorf("creating OIDC provider: %w", err)
	}

	oauth2Config := &oauth2.Config{
		ClientID: tc.ClientID,
		Endpoint: provider.Endpoint(),
		Scopes:   []string{oidc.ScopeOpenID, "profile", "email", "offline_access"},
	}

	exchangeCtx := ctx
	if insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
		httpClient := &http.Client{Transport: tr}
		exchangeCtx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}

	tokenSource := oauth2Config.TokenSource(exchangeCtx, &oauth2.Token{
		RefreshToken: tc.RefreshToken,
	})
	newToken, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("refreshing token: %w", err)
	}

	rawIDToken, ok := newToken.Extra("id_token").(string)
	if !ok {
		return fmt.Errorf("no id_token in refresh response")
	}

	tc.IDToken = rawIDToken
	tc.RefreshToken = newToken.RefreshToken
	tc.ExpiresAt = newToken.Expiry.Unix()
	return nil
}

// =============================================================================
// login command
// =============================================================================

func newLoginCommand() *cobra.Command {
	var (
		hubURL                string
		insecureSkipTLSVerify bool
		token                 string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the platform hub (OIDC or static token)",
		Long: `Authenticate with the platform hub.

Without --token, opens a browser for OIDC login (Zitadel).
With --token, uses static bearer token authentication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if hubURL == "" {
				return fmt.Errorf("--hub-url is required")
			}
			hubURL = strings.TrimRight(hubURL, "/")

			if token != "" {
				return runStaticTokenLogin(hubURL, token, insecureSkipTLSVerify)
			}
			return runOIDCLogin(cmd.Context(), hubURL, insecureSkipTLSVerify)
		},
	}

	cmd.Flags().StringVar(&hubURL, "hub-url", "", "Platform hub server URL (required)")
	cmd.Flags().BoolVar(&insecureSkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS certificate verification")
	cmd.Flags().StringVar(&token, "token", "", "Static bearer token (if omitted, OIDC browser login is used)")

	return cmd
}

func runOIDCLogin(ctx context.Context, hubURL string, insecure bool) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting callback listener: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	responseCh := make(chan *platformauth.CallbackResponse, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		responseParam := r.URL.Query().Get("response")
		if responseParam == "" {
			http.Error(w, "missing response parameter", http.StatusBadRequest)
			errCh <- fmt.Errorf("callback missing response parameter")
			return
		}

		decoded, err := base64.URLEncoding.DecodeString(responseParam)
		if err != nil {
			http.Error(w, "invalid response parameter", http.StatusBadRequest)
			errCh <- fmt.Errorf("decoding callback response: %w", err)
			return
		}

		var resp platformauth.CallbackResponse
		if err := json.Unmarshal(decoded, &resp); err != nil {
			http.Error(w, "invalid response payload", http.StatusBadRequest)
			errCh <- fmt.Errorf("parsing callback response: %w", err)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<h2>Login successful!</h2>
			<p>You can close this window and return to the terminal.</p>
			<script>window.close();</script>
		</body></html>`)

		responseCh <- &resp
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Close()

	authorizeURL := fmt.Sprintf("%s/auth/authorize?redirect_uri=%s", hubURL, callbackURL)
	fmt.Printf("Opening browser for OIDC login...\n")
	fmt.Printf("If the browser doesn't open, visit:\n  %s\n\n", authorizeURL)

	if err := openBrowser(authorizeURL); err != nil {
		klog.V(2).Infof("Failed to open browser: %v", err)
	}

	fmt.Printf("Waiting for authentication...\n")
	select {
	case resp := <-responseCh:
		return handleOIDCResponse(resp, insecure)
	case err := <-errCh:
		return fmt.Errorf("OIDC login failed: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("OIDC login timed out (5 minutes)")
	}
}

func handleOIDCResponse(resp *platformauth.CallbackResponse, insecure bool) error {
	// 1. Save tokens to cache for the get-token exec plugin.
	tc := &tokenCache{
		IDToken:      resp.IDToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    resp.ExpiresAt,
		IssuerURL:    resp.IssuerURL,
		ClientID:     resp.ClientID,
	}
	if err := saveTokenCache(tc); err != nil {
		return fmt.Errorf("saving token cache: %w", err)
	}

	// 2. Build kubeconfig with exec credential plugin and user's cluster URL.
	hubURL := resp.HubURL
	serverURL := hubURL
	if resp.ClusterName != "" {
		// ClusterName is the full kcp cluster path (e.g. "root:platform:tenants:u-abc123").
		serverURL = hubURL + "/clusters/" + resp.ClusterName
	}

	// Find the platform-cli binary path for the exec plugin.
	cliPath, err := os.Executable()
	if err != nil {
		cliPath = "platform-cli" // Fall back to PATH lookup.
	}

	config := clientcmdapi.NewConfig()

	config.Clusters["platform"] = &clientcmdapi.Cluster{
		Server:                serverURL,
		InsecureSkipTLSVerify: insecure,
	}

	execArgs := []string{
		"get-token",
		"--oidc-issuer-url=" + resp.IssuerURL,
		"--oidc-client-id=" + resp.ClientID,
	}
	if insecure {
		execArgs = append(execArgs, "--insecure-skip-tls-verify")
	}

	config.AuthInfos["platform"] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Command:    cliPath,
			Args:       execArgs,
		},
	}

	config.Contexts["platform"] = &clientcmdapi.Context{
		Cluster:  "platform",
		AuthInfo: "platform",
	}

	config.CurrentContext = "platform"

	kubeconfigBytes, err := clientcmd.Write(*config)
	if err != nil {
		return fmt.Errorf("serializing kubeconfig: %w", err)
	}

	if err := mergeKubeconfig(kubeconfigBytes); err != nil {
		return fmt.Errorf("merging kubeconfig: %w", err)
	}

	fmt.Printf("Login successful! User: %s\n", resp.Email)
	fmt.Printf("Cluster: %s\n", serverURL)
	fmt.Printf("Kubeconfig context \"platform\" has been set.\n")
	fmt.Printf("Run: kubectl --context=platform get namespaces\n")
	return nil
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

// =============================================================================
// static token login
// =============================================================================

func runStaticTokenLogin(hubURL, token string, insecure bool) error {
	client := &http.Client{Timeout: 10 * time.Second}
	if insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	req, err := http.NewRequest(http.MethodPost, hubURL+"/auth/token-login", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling token-login endpoint: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token-login failed (status %d): %s", resp.StatusCode, string(body))
	}

	var loginResp auth.LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return fmt.Errorf("parsing login response: %w", err)
	}

	if err := mergeKubeconfig(loginResp.Kubeconfig); err != nil {
		return fmt.Errorf("merging kubeconfig: %w", err)
	}

	fmt.Printf("Login successful! User: %s\n", loginResp.UserID)
	fmt.Printf("Kubeconfig context \"platform\" has been set.\n")
	fmt.Printf("Run: kubectl --context=platform get namespaces\n")
	return nil
}

// =============================================================================
// kubeconfig helpers
// =============================================================================

func mergeKubeconfig(kubeconfigBytes []byte) error {
	newConfig, err := clientcmd.Load(kubeconfigBytes)
	if err != nil {
		return fmt.Errorf("parsing received kubeconfig: %w", err)
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	existingConfig, err := loadingRules.GetStartingConfig()
	if err != nil {
		existingConfig = clientcmdapi.NewConfig()
	}

	for k, v := range newConfig.Clusters {
		existingConfig.Clusters[k] = v
	}
	for k, v := range newConfig.AuthInfos {
		existingConfig.AuthInfos[k] = v
	}
	for k, v := range newConfig.Contexts {
		existingConfig.Contexts[k] = v
	}
	existingConfig.CurrentContext = newConfig.CurrentContext

	configPath := loadingRules.GetDefaultFilename()
	if err := clientcmd.WriteToFile(*existingConfig, configPath); err != nil {
		return fmt.Errorf("writing kubeconfig to %s: %w", configPath, err)
	}

	return nil
}
