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
				AccessToken: "xoxb-test",
				ExpiresAt:   tt.expiresAt,
			}
			got := cred.IsExpired(tt.grace)
			if got != tt.want {
				t.Errorf("IsExpired(%v) = %v, want %v", tt.grace, got, tt.want)
			}
		})
	}
}

func TestSlackOAuth2Config_AuthorizationURL(t *testing.T) {
	cfg := SlackOAuth2Config("client123", "secret456", []string{"channels:read", "chat:write"})

	url := cfg.AuthorizationURL()

	if url == "" {
		t.Fatal("AuthorizationURL() returned empty string")
	}

	// Check that it contains the expected components
	checks := []string{
		"https://slack.com/oauth/v2/authorize",
		"client_id=client123",
		"scope=channels%3Aread+chat%3Awrite",
		"redirect_uri=http%3A%2F%2Flocalhost%3A8976%2Fcallback",
	}

	for _, check := range checks {
		if !strings.Contains(url, check) {
			t.Errorf("AuthorizationURL() missing %q\ngot: %s", check, url)
		}
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
