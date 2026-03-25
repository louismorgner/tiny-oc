package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth2Config holds the configuration for an OAuth2 flow.
type OAuth2Config struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	Scopes       []string
	RedirectPort int
}

// OAuth2TokenResponse represents the response from a token exchange.
type OAuth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	OK           bool   `json:"ok"`
	Error        string `json:"error"`
}

// SlackOAuth2Config returns a pre-configured OAuth2Config for Slack.
func SlackOAuth2Config(clientID, clientSecret string, scopes []string) *OAuth2Config {
	return &OAuth2Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      "https://slack.com/oauth/v2/authorize",
		TokenURL:     "https://slack.com/api/oauth.v2.access",
		Scopes:       scopes,
		RedirectPort: 8976,
	}
}

// AuthorizationURL builds the URL to redirect the user to for OAuth consent.
func (c *OAuth2Config) AuthorizationURL() string {
	params := url.Values{
		"client_id":    {c.ClientID},
		"scope":        {strings.Join(c.Scopes, " ")},
		"redirect_uri": {fmt.Sprintf("http://localhost:%d/callback", c.RedirectPort)},
	}
	return c.AuthURL + "?" + params.Encode()
}

// RunCallbackServer starts a local HTTP server, waits for the OAuth callback,
// and returns the authorization code. The server shuts down after receiving the code.
func (c *OAuth2Config) RunCallbackServer(ctx context.Context) (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- fmt.Errorf("OAuth error: %s — %s", errMsg, r.URL.Query().Get("error_description"))
			fmt.Fprintf(w, "<html><body><h2>Authorization failed</h2><p>%s</p><p>You can close this tab.</p></body></html>", errMsg)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no authorization code in callback")
			fmt.Fprint(w, "<html><body><h2>Error</h2><p>No authorization code received. You can close this tab.</p></body></html>")
			return
		}

		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", c.RedirectPort))
	if err != nil {
		return "", fmt.Errorf("failed to start callback server on port %d: %w", c.RedirectPort, err)
	}

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Shutdown(context.Background())

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ExchangeCode exchanges an authorization code for access and refresh tokens.
func (c *OAuth2Config) ExchangeCode(code string) (*Credential, error) {
	data := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"redirect_uri":  {fmt.Sprintf("http://localhost:%d/callback", c.RedirectPort)},
	}

	resp, err := http.PostForm(c.TokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp OAuth2TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if !tokenResp.OK {
		return nil, fmt.Errorf("token exchange failed: %s", tokenResp.Error)
	}

	cred := &Credential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
	}

	if tokenResp.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		cred.ExpiresAt = &expiresAt
	}

	return cred, nil
}

// RefreshAccessToken uses a refresh token to obtain a new access token.
func RefreshAccessToken(tokenURL, clientID, clientSecret, refreshToken string) (*Credential, error) {
	data := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	var tokenResp OAuth2TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	if !tokenResp.OK && tokenResp.AccessToken == "" {
		errMsg := tokenResp.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return nil, fmt.Errorf("token refresh failed: %s", errMsg)
	}

	cred := &Credential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
	}

	// Keep the old refresh token if a new one wasn't issued
	if cred.RefreshToken == "" {
		cred.RefreshToken = refreshToken
	}

	if tokenResp.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		cred.ExpiresAt = &expiresAt
	}

	return cred, nil
}

// IsExpired checks if the credential's access token has expired or will expire
// within the given grace period.
func (c *Credential) IsExpired(grace time.Duration) bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().Add(grace).After(*c.ExpiresAt)
}
