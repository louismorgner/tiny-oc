package naming

import "testing"

func TestFromPrompt(t *testing.T) {
	tests := []struct {
		prompt string
		want   string
	}{
		{"", ""},
		{"  ", ""},
		{"Fix the auth token refresh bug", "fix the auth token refresh bug"},
		{"Fix the auth token refresh bug in the OAuth handler for multi-tenant environments", "fix the auth token refresh bug"},
		{"IMPLEMENT user search\nMore details here", "implement user search"},
		{`"refactor database queries"`, "refactor database queries"},
		{"add logging.", "add logging"},
	}

	for _, tt := range tests {
		got := FromPrompt(tt.prompt)
		if got != tt.want {
			t.Errorf("FromPrompt(%q) = %q, want %q", tt.prompt, got, tt.want)
		}
	}
}
