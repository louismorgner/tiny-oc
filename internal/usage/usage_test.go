package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	content := `{"type":"user","message":{"role":"user","content":"hello"}}
{"type":"assistant","message":{"role":"assistant","content":"hi","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":200,"cache_creation_input_tokens":300}}}
{"type":"assistant","message":{"role":"assistant","content":"more","usage":{"input_tokens":150,"output_tokens":75,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	u := parseJSONL(path)

	if u.InputTokens != 250 {
		t.Errorf("InputTokens = %d, want 250", u.InputTokens)
	}
	if u.OutputTokens != 125 {
		t.Errorf("OutputTokens = %d, want 125", u.OutputTokens)
	}
	if u.CacheRead != 200 {
		t.Errorf("CacheRead = %d, want 200", u.CacheRead)
	}
	if u.CacheCreate != 300 {
		t.Errorf("CacheCreate = %d, want 300", u.CacheCreate)
	}
	if u.Total() != 875 {
		t.Errorf("Total = %d, want 875", u.Total())
	}
}

func TestFormatTotal(t *testing.T) {
	tests := []struct {
		usage TokenUsage
		want  string
	}{
		{TokenUsage{}, ""},
		{TokenUsage{InputTokens: 500}, "500 tokens"},
		{TokenUsage{InputTokens: 1500}, "1.5k tokens"},
		{TokenUsage{InputTokens: 1_500_000}, "1.5M tokens"},
	}
	for _, tt := range tests {
		got := tt.usage.FormatTotal()
		if got != tt.want {
			t.Errorf("FormatTotal() = %q, want %q", got, tt.want)
		}
	}
}

func TestParseJSONL_FallbackScanAll(t *testing.T) {
	dir := t.TempDir()

	// Write a JSONL file with a different name than the session ID
	content := `{"type":"assistant","message":{"role":"assistant","content":"hi","usage":{"input_tokens":500,"output_tokens":200}}}
`
	if err := os.WriteFile(filepath.Join(dir, "other-uuid.jsonl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// parseJSONL for a non-matching session ID returns zero
	u := parseJSONL(filepath.Join(dir, "my-session.jsonl"))
	if u.Total() != 0 {
		t.Errorf("expected zero for non-matching session, got %d", u.Total())
	}

	// But scanning all JSONL files in the dir should find it
	entries, _ := os.ReadDir(dir)
	var total TokenUsage
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		tok := parseJSONL(filepath.Join(dir, e.Name()))
		total.InputTokens += tok.InputTokens
		total.OutputTokens += tok.OutputTokens
	}
	if total.Total() != 700 {
		t.Errorf("expected 700 tokens from fallback scan, got %d", total.Total())
	}
}

func TestParseJSONL_MissingFile(t *testing.T) {
	u := parseJSONL("/nonexistent/file.jsonl")
	if u.Total() != 0 {
		t.Errorf("expected zero usage for missing file, got %d", u.Total())
	}
}

func TestClaudeProjectDir(t *testing.T) {
	home, _ := os.UserHomeDir()

	dir := claudeProjectDir("/private/tmp/toc-sessions/buddy-123")
	want := filepath.Join(home, ".claude", "projects", "-private-tmp-toc-sessions-buddy-123")
	if dir != want {
		t.Errorf("claudeProjectDir = %q, want %q", dir, want)
	}

	// Paths with dots (hidden dirs) should have dots removed
	dir2 := claudeProjectDir("/Users/test/.myapp/sessions/abc")
	want2 := filepath.Join(home, ".claude", "projects", "-Users-test-myapp-sessions-abc")
	if dir2 != want2 {
		t.Errorf("claudeProjectDir (dots) = %q, want %q", dir2, want2)
	}
}
