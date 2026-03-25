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
	Scopes       []string // bot scopes — sent as "scope" param
	UserScopes   []string // user scopes — sent as "user_scope" param (Slack user tokens)
	RedirectPort int
}

// OAuth2TokenResponse represents the response from a token exchange.
// For Slack V2 OAuth, user tokens are nested under authed_user.
type OAuth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	OK           bool   `json:"ok"`
	Error        string `json:"error"`
	AuthedUser   *struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	} `json:"authed_user,omitempty"`
}

// SlackOAuth2Config returns a pre-configured OAuth2Config for Slack.
// When userScopes is non-empty, the flow requests user tokens (xoxp) via user_scope.
// When scopes is non-empty, bot tokens (xoxb) are also requested via scope.
func SlackOAuth2Config(clientID, clientSecret string, scopes, userScopes []string) *OAuth2Config {
	return &OAuth2Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      "https://slack.com/oauth/v2/authorize",
		TokenURL:     "https://slack.com/api/oauth.v2.access",
		Scopes:       scopes,
		UserScopes:   userScopes,
		RedirectPort: 8976,
	}
}

// RedirectURI returns the localhost callback URI for this config.
func (c *OAuth2Config) RedirectURI() string {
	return fmt.Sprintf("http://localhost:%d/callback", c.RedirectPort)
}

// AuthorizationURL builds the URL to redirect the user to for OAuth consent.
// For Slack user tokens, scopes are sent as "user_scope" (comma-separated).
func (c *OAuth2Config) AuthorizationURL() string {
	params := url.Values{
		"client_id":    {c.ClientID},
		"redirect_uri": {c.RedirectURI()},
	}
	if len(c.Scopes) > 0 {
		params.Set("scope", strings.Join(c.Scopes, ","))
	}
	if len(c.UserScopes) > 0 {
		params.Set("user_scope", strings.Join(c.UserScopes, ","))
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
// For Slack, when UserScopes were requested, the user token is extracted from
// the authed_user field in the response.
func (c *OAuth2Config) ExchangeCode(code string) (*Credential, error) {
	data := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.RedirectURI()},
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

	// Prefer user token from authed_user when user_scopes were requested.
	accessToken := tokenResp.AccessToken
	if len(c.UserScopes) > 0 && tokenResp.AuthedUser != nil && tokenResp.AuthedUser.AccessToken != "" {
		accessToken = tokenResp.AuthedUser.AccessToken
	}

	if accessToken == "" {
		return nil, fmt.Errorf("token exchange returned empty access token")
	}

	cred := &Credential{
		AccessToken:  accessToken,
		RefreshToken: tokenResp.RefreshToken,
	}

	if tokenResp.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		cred.ExpiresAt = &expiresAt
	}

	return cred, nil
}

// ParseCodeFromURL extracts the authorization code from a callback URL.
// Accepts either a full URL (http://localhost:8976/callback?code=xyz) or a bare code string.
func ParseCodeFromURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty input")
	}

	// If it looks like a URL, parse the code param
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid URL: %w", err)
		}
		if errMsg := u.Query().Get("error"); errMsg != "" {
			return "", fmt.Errorf("OAuth error: %s — %s", errMsg, u.Query().Get("error_description"))
		}
		code := u.Query().Get("code")
		if code == "" {
			return "", fmt.Errorf("no authorization code found in URL")
		}
		return code, nil
	}

	// Otherwise treat the whole string as the code
	return raw, nil
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
