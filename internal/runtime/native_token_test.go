package runtime

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hi", 1},              // 2 bytes / 4 = 0.5, rounds up to 1
		{"hello world", 3},     // 11 bytes / 4 = 2.75, rounds up to 3
		{strings.Repeat("a", 100), 25}, // 100 / 4 = 25
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input[:min(len(tt.input), 20)], got, tt.want)
		}
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: strings.Repeat("x", 400)}, // 100 tokens + 4 overhead
		{Role: "user", Content: "hello"},                     // ~1 token + 4
	}
	got := estimateMessagesTokens(msgs)
	// 100 + 4 + 2 + 4 = 110 (approximately)
	if got < 100 || got > 120 {
		t.Errorf("estimateMessagesTokens = %d, expected ~110", got)
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
