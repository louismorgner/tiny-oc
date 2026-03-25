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

func TestParseJSONL_MissingFile(t *testing.T) {
	u := parseJSONL("/nonexistent/file.jsonl")
	if u.Total() != 0 {
		t.Errorf("expected zero usage for missing file, got %d", u.Total())
	}
}

func TestClaudeProjectDir(t *testing.T) {
	dir := claudeProjectDir("/private/tmp/toc-sessions/buddy-123")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".claude", "projects", "-private-tmp-toc-sessions-buddy-123")
	if dir != want {
		t.Errorf("claudeProjectDir = %q, want %q", dir, want)
	}
}
