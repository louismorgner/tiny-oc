package integration

import (
	"fmt"
	"strings"
	"testing"
)

func TestInvokeError_Error(t *testing.T) {
	t.Run("with fix", func(t *testing.T) {
		err := NewPermissionDeniedError("myagent", "slack", "send_message")
		msg := err.Error()
		if !strings.Contains(msg, "myagent") {
			t.Errorf("expected agent name in error, got: %s", msg)
		}
		if !strings.Contains(msg, "slack.send_message") {
			t.Errorf("expected integration.action in error, got: %s", msg)
		}
		if !strings.Contains(msg, "Fix:") {
			t.Errorf("expected Fix line in error, got: %s", msg)
		}
		if !strings.Contains(msg, "oc-agent.yaml") {
			t.Errorf("expected oc-agent.yaml reference in fix, got: %s", msg)
		}
	})

	t.Run("without fix", func(t *testing.T) {
		err := &InvokeError{
			Kind:    ErrAPIError,
			Message: "something went wrong",
		}
		if err.Error() != "something went wrong" {
			t.Errorf("got %q, want %q", err.Error(), "something went wrong")
		}
	})
}

func TestNewNoIntegrationPermError(t *testing.T) {
	err := NewNoIntegrationPermError("bot", "github")
	if err.Kind != ErrPermissionDenied {
		t.Errorf("kind = %v, want ErrPermissionDenied", err.Kind)
	}
	if !strings.Contains(err.Error(), "bot") {
		t.Error("expected agent name in error")
	}
	if !strings.Contains(err.Error(), "github") {
		t.Error("expected integration name in error")
	}
}

func TestNewActionNotFoundError(t *testing.T) {
	err := NewActionNotFoundError("slack", "read_dms", []string{"send_message", "read_messages", "list_channels"})
	if err.Kind != ErrActionNotFound {
		t.Errorf("kind = %v, want ErrActionNotFound", err.Kind)
	}
	msg := err.Error()
	if !strings.Contains(msg, "read_dms") {
		t.Error("expected action name in error")
	}
	if !strings.Contains(msg, "send_message") {
		t.Error("expected available actions in error")
	}
	if !strings.Contains(msg, "toc runtime invoke slack --help") {
		t.Error("expected help command in fix")
	}
}

func TestNewMissingScopeError(t *testing.T) {
	err := NewMissingScopeError("slack", "chat:write")
	if err.Kind != ErrMissingOAuthScope {
		t.Errorf("kind = %v, want ErrMissingOAuthScope", err.Kind)
	}
	msg := err.Error()
	if !strings.Contains(msg, "chat:write") {
		t.Error("expected scope in error")
	}
	if !strings.Contains(msg, "toc integrate add slack") {
		t.Error("expected integrate add command in fix")
	}
}

func TestNewCredentialError(t *testing.T) {
	err := NewCredentialError("slack", fmt.Errorf("file not found"))
	if err.Kind != ErrCredentialError {
		t.Errorf("kind = %v, want ErrCredentialError", err.Kind)
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Error("expected underlying error in message")
	}
}

func TestClassifySlackError(t *testing.T) {
	tests := []struct {
		slackErr string
		action   string
		wantKind InvokeErrorKind
		wantSub  string // substring expected in error message
	}{
		{"missing_scope", "send_message", ErrMissingOAuthScope, "chat:write"},
		{"missing_scope", "read_messages", ErrMissingOAuthScope, "channels:history"},
		{"not_authed", "send_message", ErrCredentialError, "not_authed"},
		{"invalid_auth", "send_message", ErrCredentialError, "invalid_auth"},
		{"token_revoked", "send_message", ErrCredentialError, "token_revoked"},
		{"channel_not_found", "read_messages", ErrAPIError, "channel_not_found"},
		{"no_permission", "send_message", ErrMissingOAuthScope, "chat:write"},
		{"some_unknown_error", "send_message", ErrAPIError, "some_unknown_error"},
	}

	for _, tt := range tests {
		t.Run(tt.slackErr+"_"+tt.action, func(t *testing.T) {
			err := ClassifySlackError(tt.slackErr, tt.action)
			if err.Kind != tt.wantKind {
				t.Errorf("kind = %v, want %v", err.Kind, tt.wantKind)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantSub)
			}
		})
	}
}
