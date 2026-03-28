package usage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
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

func TestFormatBreakdown(t *testing.T) {
	tests := []struct {
		name  string
		usage TokenUsage
		want  string
	}{
		{"zero", TokenUsage{}, ""},
		{"small", TokenUsage{InputTokens: 500, OutputTokens: 123}, "500 input / 123 output"},
		{"thousands", TokenUsage{InputTokens: 12345, OutputTokens: 3456}, "12k input / 3k output"},
		{"millions", TokenUsage{InputTokens: 1_500_000, OutputTokens: 250_000}, "1.5M input / 250k output"},
		{"input only", TokenUsage{InputTokens: 100}, "100 input / 0 output"},
		{"output only", TokenUsage{OutputTokens: 200}, "0 input / 200 output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.usage.FormatBreakdown()
			if got != tt.want {
				t.Errorf("FormatBreakdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatWithCommas(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{1234567, "1,234,567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatWithCommas(tt.n)
			if got != tt.want {
				t.Errorf("formatWithCommas(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
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

func TestForSession_NativeStateUsage(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-native",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}
	if err := runtime.SaveState(sess, &runtime.State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Agent:     "native-agent",
		Usage: runtime.TokenUsageSnapshot{
			InputTokens:  111,
			OutputTokens: 222,
			CacheRead:    333,
			CacheCreate:  444,
		},
	}); err != nil {
		t.Fatal(err)
	}

	u := ForSession(sess)
	if u.InputTokens != 111 || u.OutputTokens != 222 || u.CacheRead != 333 || u.CacheCreate != 444 {
		t.Fatalf("usage = %#v", u)
	}
}

func TestParseCodexJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codex.jsonl")

	content := `{"type":"thread.started","thread_id":"codex-thread-1"}
{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":40}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":5,"output_tokens":4}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	u := parseCodexJSONL(path)
	if u.InputTokens != 110 {
		t.Fatalf("InputTokens = %d", u.InputTokens)
	}
	if u.CacheRead != 30 {
		t.Fatalf("CacheRead = %d", u.CacheRead)
	}
	if u.OutputTokens != 44 {
		t.Fatalf("OutputTokens = %d", u.OutputTokens)
	}
}

func TestParseCodexJSONL_PrefersRicherMixedTotals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codex-mixed.jsonl")

	content := `{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":40}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":150,"cached_input_tokens":30,"output_tokens":60}}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	u := parseCodexJSONL(path)
	if u.InputTokens != 150 {
		t.Fatalf("InputTokens = %d", u.InputTokens)
	}
	if u.CacheRead != 30 {
		t.Fatalf("CacheRead = %d", u.CacheRead)
	}
	if u.OutputTokens != 60 {
		t.Fatalf("OutputTokens = %d", u.OutputTokens)
	}
}
