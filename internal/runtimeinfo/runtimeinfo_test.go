package runtimeinfo

import "testing"

func TestClaudeCodeModelOptionsIncludeDefault(t *testing.T) {
	options := ModelOptions(DefaultRuntime)
	if len(options) == 0 {
		t.Fatal("ModelOptions() returned no options")
	}
	if options[0].ID != "default" {
		t.Fatalf("ModelOptions()[0].ID = %q, want %q", options[0].ID, "default")
	}
}

func TestValidateModelSelection_ClaudeCodeAcceptsDefault(t *testing.T) {
	if err := ValidateModelSelection(DefaultRuntime, "default", false); err != nil {
		t.Fatalf("expected default to be accepted for claude-code, got %v", err)
	}
}
