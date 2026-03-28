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

func TestCodexRuntimeDefaults(t *testing.T) {
	if err := ValidateRuntime(CodexRuntime); err != nil {
		t.Fatalf("expected codex runtime to be supported, got %v", err)
	}
	if got := DefaultModel(CodexRuntime); got != "gpt-5-codex" {
		t.Fatalf("DefaultModel(codex) = %q", got)
	}
	if err := ValidateModelSelection(CodexRuntime, "gpt-5-codex", false); err != nil {
		t.Fatalf("expected codex model to validate, got %v", err)
	}
	if err := ValidateModelSelection(CodexRuntime, "", false); err == nil {
		t.Fatal("expected empty codex model to fail validation")
	}
}
