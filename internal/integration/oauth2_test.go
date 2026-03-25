package integration

import (
	"strings"
	"testing"
	"time"
)

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt *time.Time
		grace     time.Duration
		want      bool
	}{
		{
			name:      "nil expiry — never expires",
			expiresAt: nil,
			grace:     30 * time.Second,
			want:      false,
		},
		{
			name:      "future expiry — not expired",
			expiresAt: timePtr(time.Now().Add(10 * time.Minute)),
			grace:     30 * time.Second,
			want:      false,
		},
		{
			name:      "past expiry — expired",
			expiresAt: timePtr(time.Now().Add(-5 * time.Minute)),
			grace:     30 * time.Second,
			want:      true,
		},
		{
			name:      "within grace period — expired",
			expiresAt: timePtr(time.Now().Add(15 * time.Second)),
			grace:     30 * time.Second,
			want:      true,
		},
		{
			name:      "just outside grace — not expired",
			expiresAt: timePtr(time.Now().Add(2 * time.Minute)),
			grace:     30 * time.Second,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cred := &Credential{
				AccessToken: "xoxp-test",
				ExpiresAt:   tt.expiresAt,
			}
			got := cred.IsExpired(tt.grace)
			if got != tt.want {
				t.Errorf("IsExpired(%v) = %v, want %v", tt.grace, got, tt.want)
			}
		})
	}
}

func TestSlackOAuth2Config_AuthorizationURL_UserScopes(t *testing.T) {
	cfg := SlackOAuth2Config("client123", "secret456", nil, []string{"channels:read", "chat:write", "search:read"})

	url := cfg.AuthorizationURL()

	if url == "" {
		t.Fatal("AuthorizationURL() returned empty string")
	}

	checks := []string{
		"https://slack.com/oauth/v2/authorize",
		"client_id=client123",
		"user_scope=channels%3Aread%2Cchat%3Awrite%2Csearch%3Aread",
		"redirect_uri=http%3A%2F%2Flocalhost%3A8976%2Fcallback",
	}

	for _, check := range checks {
		if !strings.Contains(url, check) {
			t.Errorf("AuthorizationURL() missing %q\ngot: %s", check, url)
		}
	}

	// Should NOT contain a bare "scope=" param (not "user_scope=") when only user_scopes are set
	// Split on & and check each param individually
	for _, part := range strings.Split(strings.SplitN(url, "?", 2)[1], "&") {
		if strings.HasPrefix(part, "scope=") {
			t.Errorf("AuthorizationURL() should not contain bot scope param when only user_scopes set\ngot param: %s\nfull url: %s", part, url)
		}
	}
}

func TestSlackOAuth2Config_AuthorizationURL_BothScopes(t *testing.T) {
	cfg := SlackOAuth2Config("client123", "secret456",
		[]string{"commands"},
		[]string{"channels:read", "chat:write"},
	)

	url := cfg.AuthorizationURL()

	if !strings.Contains(url, "scope=commands") {
		t.Errorf("AuthorizationURL() missing bot scope param\ngot: %s", url)
	}
	if !strings.Contains(url, "user_scope=channels%3Aread%2Cchat%3Awrite") {
		t.Errorf("AuthorizationURL() missing user_scope param\ngot: %s", url)
	}
}

func TestParseCodeFromURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "full callback URL",
			input: "http://localhost:8976/callback?code=abc123&state=xyz",
			want:  "abc123",
		},
		{
			name:  "bare code string",
			input: "abc123",
			want:  "abc123",
		},
		{
			name:  "bare code with whitespace",
			input: "  abc123  ",
			want:  "abc123",
		},
		{
			name:    "URL with error",
			input:   "http://localhost:8976/callback?error=access_denied&error_description=user+denied",
			wantErr: true,
		},
		{
			name:    "URL without code param",
			input:   "http://localhost:8976/callback?state=xyz",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCodeFromURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCodeFromURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseCodeFromURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
