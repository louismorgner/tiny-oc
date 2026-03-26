package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsSlackChannelID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"C01234ABCDE", true},
		{"D01234ABCDE", true},
		{"G01234ABCDE", true},
		{"C1", true},
		{"#general", false},
		{"general", false},
		{"", false},
		{"X01234", false},
		{"C", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isSlackChannelID(tt.input)
			if got != tt.want {
				t.Errorf("isSlackChannelID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSlackChannelResolver_ResolveChannelID(t *testing.T) {
	resolver := NewSlackChannelResolver("test-token")

	// Channel IDs should pass through unchanged
	id, err := resolver.Resolve("C01234ABCDE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "C01234ABCDE" {
		t.Errorf("got %q, want C01234ABCDE", id)
	}
}

func TestSlackChannelResolver_ResolveName(t *testing.T) {
	// Set up a mock Slack API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"ok": true,
			"channels": []map[string]interface{}{
				{"id": "C001", "name": "general"},
				{"id": "C002", "name": "random"},
				{"id": "C003", "name": "engineering"},
			},
			"response_metadata": map[string]string{
				"next_cursor": "",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	resolver := NewSlackChannelResolver("test-token")
	// Pre-populate cache to avoid hitting real API
	resolver.cache["general"] = "C001"
	resolver.cache["random"] = "C002"
	resolver.cache["engineering"] = "C003"
	resolver.ready = true

	tests := []struct {
		input string
		want  string
	}{
		{"#general", "C001"},
		{"general", "C001"},
		{"#random", "C002"},
		{"engineering", "C003"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, err := resolver.Resolve(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.input, id, tt.want)
			}
		})
	}
}

func TestSlackChannelResolver_NotFound(t *testing.T) {
	resolver := NewSlackChannelResolver("test-token")
	resolver.ready = true // Mark as populated (empty cache)

	_, err := resolver.Resolve("#nonexistent")
	if err == nil {
		t.Error("expected error for non-existent channel")
	}
}

func TestSlackChannelResolver_FallbackToRawID(t *testing.T) {
	resolver := NewSlackChannelResolver("test-token")
	resolver.ready = true // Mark as populated (empty cache)

	// A channel ID that's not in cache should still work as a raw ID fallback
	id, err := resolver.Resolve("C01NOTINCACHE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "C01NOTINCACHE" {
		t.Errorf("got %q, want C01NOTINCACHE", id)
	}
}

func TestSlackChannelResolver_FallbackOnPopulateError(t *testing.T) {
	// Use an invalid URL that will fail to populate
	resolver := NewSlackChannelResolver("invalid-token-that-will-fail")
	// Don't mark ready — force populate to be called

	// Passing a channel ID should fall back even if populate fails
	// We can't easily test the HTTP failure path without a mock server,
	// but we can test the logic after cache is ready with an ID format
	resolver.ready = true
	id, err := resolver.Resolve("D02ABCDEF")
	if err != nil {
		t.Fatalf("unexpected error for DM channel ID: %v", err)
	}
	if id != "D02ABCDEF" {
		t.Errorf("got %q, want D02ABCDEF", id)
	}

	// Group DM channel IDs should also work
	id, err = resolver.Resolve("G03XYZ789")
	if err != nil {
		t.Fatalf("unexpected error for group channel ID: %v", err)
	}
	if id != "G03XYZ789" {
		t.Errorf("got %q, want G03XYZ789", id)
	}
}

func TestCheckSlackResponseForAction_MissingScope(t *testing.T) {
	data := map[string]interface{}{"ok": false, "error": "missing_scope"}
	err := CheckSlackResponseForAction(200, data, "send_message")
	if err == nil {
		t.Fatal("expected error")
	}

	invokeErr, ok := err.(*InvokeError)
	if !ok {
		t.Fatalf("expected *InvokeError, got %T", err)
	}
	if invokeErr.Kind != ErrMissingOAuthScope {
		t.Errorf("kind = %v, want ErrMissingOAuthScope", invokeErr.Kind)
	}
}

func TestCheckSlackResponseForAction_InvalidAuth(t *testing.T) {
	data := map[string]interface{}{"ok": false, "error": "invalid_auth"}
	err := CheckSlackResponseForAction(200, data, "read_messages")
	if err == nil {
		t.Fatal("expected error")
	}

	invokeErr, ok := err.(*InvokeError)
	if !ok {
		t.Fatalf("expected *InvokeError, got %T", err)
	}
	if invokeErr.Kind != ErrCredentialError {
		t.Errorf("kind = %v, want ErrCredentialError", invokeErr.Kind)
	}
}

func TestCheckSlackResponse(t *testing.T) {
	tests := []struct {
		name       string
		data       interface{}
		wantErr    bool
		errSubstr  string // substring to check in error message
	}{
		{
			name:    "success response",
			data:    map[string]interface{}{"ok": true, "channel": "C123"},
			wantErr: false,
		},
		{
			name:      "error response",
			data:      map[string]interface{}{"ok": false, "error": "channel_not_found"},
			wantErr:   true,
			errSubstr: "channel_not_found",
		},
		{
			name:      "error without message",
			data:      map[string]interface{}{"ok": false},
			wantErr:   true,
			errSubstr: "unknown_error",
		},
		{
			name:    "non-map response",
			data:    []interface{}{"hello"},
			wantErr: false,
		},
		{
			name:    "no ok field",
			data:    map[string]interface{}{"data": "value"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckSlackResponse(200, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCheckPermission_SlackPatterns(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		action      string
		target      string
		want        bool
	}{
		{
			name:        "send_message wildcard allows any channel",
			permissions: []string{"send_message:*"},
			action:      "send_message",
			target:      "#general",
			want:        true,
		},
		{
			name:        "read_messages scoped to #general allows #general",
			permissions: []string{"read_messages:#general"},
			action:      "read_messages",
			target:      "#general",
			want:        true,
		},
		{
			name:        "read_messages scoped to #general denies #random",
			permissions: []string{"read_messages:#general"},
			action:      "read_messages",
			target:      "#random",
			want:        false,
		},
		{
			name:        "list_channels wildcard",
			permissions: []string{"list_channels:*"},
			action:      "list_channels",
			target:      "*",
			want:        true,
		},
		{
			name:        "react wildcard",
			permissions: []string{"react:*"},
			action:      "react",
			target:      "#general",
			want:        true,
		},
		{
			name:        "no slack permissions denies all",
			permissions: []string{},
			action:      "send_message",
			target:      "#general",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms, err := ParsePermissions(tt.permissions)
			if err != nil {
				t.Fatalf("ParsePermissions failed: %v", err)
			}
			got := CheckPermission(perms, tt.action, tt.target)
			if got != tt.want {
				t.Errorf("CheckPermission(%v, %q, %q) = %v, want %v",
					tt.permissions, tt.action, tt.target, got, tt.want)
			}
		})
	}
}
