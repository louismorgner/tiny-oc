package runtime

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
	}{
		{""},
		{"hi"},
		{"hello world"},
		{strings.Repeat("a", 100)},
		{"func main() { fmt.Println(\"hello\") }"},
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if tt.input == "" {
			if got != 0 {
				t.Errorf("estimateTokens(%q) = %d, want 0", tt.input, got)
			}
			continue
		}
		if got <= 0 {
			t.Errorf("estimateTokens(%q) = %d, want > 0", tt.input[:min(len(tt.input), 20)], got)
		}
	}
}

func TestEstimateTokens_UsesRealTokenizer(t *testing.T) {
	// The real tokenizer should give different results than len/4 for this string.
	// "hello world" is 2 tokens in cl100k_base, but len/4 rounds to 3.
	got := estimateTokens("hello world")
	if codec := getTokenCodec(); codec != nil {
		if got == 3 {
			t.Error("estimateTokens(\"hello world\") = 3, looks like fallback heuristic is being used instead of real tokenizer")
		}
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: strings.Repeat("x", 400)},
		{Role: "user", Content: "hello"},
	}
	got := estimateMessagesTokens(msgs)
	// With real tokenizer or heuristic, should be a reasonable positive number.
	// The per-message overhead (4 tokens each × 2 messages = 8) is always added.
	if got < 10 {
		t.Errorf("estimateMessagesTokens = %d, expected > 10", got)
	}
}

func TestTruncateMiddle_ShortString(t *testing.T) {
	s := "short string"
	got := truncateMiddle(s, 100)
	if got != s {
		t.Errorf("truncateMiddle should not modify short strings, got %q", got)
	}
}

func TestTruncateMiddle_PreservesPrefixAndSuffix(t *testing.T) {
	// Build a string with identifiable prefix and suffix
	prefix := "=== PREFIX START ===\n"
	middle := strings.Repeat("middle content line\n", 100)
	suffix := "\n=== SUFFIX END ==="
	s := prefix + middle + suffix

	got := truncateMiddle(s, 200)
	if len(got) > 280 { // 200 + marker overhead
		t.Errorf("truncated length %d exceeds budget+marker", len(got))
	}
	if !strings.Contains(got, "PREFIX START") {
		t.Error("truncateMiddle should preserve prefix")
	}
	if !strings.Contains(got, "SUFFIX END") {
		t.Error("truncateMiddle should preserve suffix")
	}
	if !strings.Contains(got, "truncated") {
		t.Error("truncateMiddle should include truncation marker")
	}
}

func TestTruncateMiddle_MarkerIncludesStats(t *testing.T) {
	s := strings.Repeat("x", 1000)
	got := truncateMiddle(s, 200)
	if !strings.Contains(got, "bytes") {
		t.Error("marker should include byte count")
	}
	if !strings.Contains(got, "tokens") {
		t.Error("marker should include token estimate")
	}
}

func TestToolOutputBudget_KnownTools(t *testing.T) {
	if b := toolOutputBudget("Read"); b != 48*1024 {
		t.Errorf("Read budget = %d, want %d", b, 48*1024)
	}
	if b := toolOutputBudget("Grep"); b != 16*1024 {
		t.Errorf("Grep budget = %d, want %d", b, 16*1024)
	}
	if b := toolOutputBudget("Glob"); b != 8*1024 {
		t.Errorf("Glob budget = %d, want %d", b, 8*1024)
	}
}

func TestToolOutputBudget_UnknownTool(t *testing.T) {
	if b := toolOutputBudget("Unknown"); b != defaultToolOutputBudget {
		t.Errorf("Unknown tool budget = %d, want %d", b, defaultToolOutputBudget)
	}
}

func TestTruncateToolOutput_SmallOutput(t *testing.T) {
	output := "small output"
	got := truncateToolOutput("Read", output)
	if got != output {
		t.Errorf("small output should not be truncated")
	}
}

func TestTruncateToolOutput_LargeOutput(t *testing.T) {
	output := strings.Repeat("line of grep output\n", 5000) // ~100KB
	got := truncateToolOutput("Grep", output)
	budget := toolOutputBudget("Grep")
	// Allow marker overhead
	if len(got) > budget+200 {
		t.Errorf("truncated Grep output %d bytes exceeds budget %d + overhead", len(got), budget)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
