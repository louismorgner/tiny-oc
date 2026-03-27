package runtime

import (
	"strings"
	"testing"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestCompactMessages_PreservesSystemAndRecentTail(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "first request"},
		{Role: "assistant", Content: "first answer"},
		{Role: "tool", Name: "Read", Content: "file contents"},
		{Role: "user", Content: "second request"},
		{Role: "assistant", Content: "second answer"},
	}

	compacted, compactedCount, _ := compactMessages(messages, 2, 512)
	if compactedCount != 3 {
		t.Fatalf("compactedCount = %d, want 3", compactedCount)
	}
	if len(compacted) != 4 {
		t.Fatalf("len(compacted) = %d, want 4", len(compacted))
	}
	if compacted[0].Role != "system" || compacted[0].Content != "system prompt" {
		t.Fatalf("unexpected preserved system message: %#v", compacted[0])
	}
	if !isCompactionSummary(compacted[1]) {
		t.Fatalf("expected compaction summary, got %#v", compacted[1])
	}
	if compacted[1].Role != "user" {
		t.Fatalf("compaction summary should use user role for provider compatibility, got %q", compacted[1].Role)
	}
	if compacted[2].Content != "second request" || compacted[3].Content != "second answer" {
		t.Fatalf("unexpected preserved tail: %#v", compacted[2:])
	}
}

func TestIsCompactionSummary_BackwardsCompatible(t *testing.T) {
	// New format uses "user" role.
	newFmt := Message{Role: "user", Content: "[toc-summary]\nsome context"}
	if !isCompactionSummary(newFmt) {
		t.Fatal("should recognize user-role compaction summary")
	}
	// Old format used "system" role — must still be recognized for resumed sessions.
	oldFmt := Message{Role: "system", Content: "[toc-summary]\nsome context"}
	if !isCompactionSummary(oldFmt) {
		t.Fatal("should recognize legacy system-role compaction summary")
	}
	// Regular messages should not match.
	regular := Message{Role: "user", Content: "hello world"}
	if isCompactionSummary(regular) {
		t.Fatal("regular user message should not be a compaction summary")
	}
}

func TestMaybeCompactState_UpdatesStateAndWritesEvent(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-compact",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}
	state := &State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: strings.Repeat("a", 200)},
			{Role: "assistant", Content: strings.Repeat("b", 200)},
			{Role: "tool", Name: "Read", Content: strings.Repeat("c", 200)},
			{Role: "user", Content: "keep me"},
			{Role: "assistant", Content: "latest reply"},
		},
	}

	compacted, err := maybeCompactState(state, sess, &SessionConfig{
		RuntimeConfig: SessionRuntimeOptions{
			CompactionTriggerChars:    128,
			CompactionKeepRecent:      2,
			CompactionMaxSummaryChars: 512,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !compacted {
		t.Fatal("expected state to compact")
	}
	if state.CompactionCount != 1 || state.CompactedMessages != 3 {
		t.Fatalf("unexpected compaction metadata: %#v", state)
	}
	if state.LastCompactedAt.IsZero() {
		t.Fatalf("expected LastCompactedAt to be set: %#v", state)
	}
	if len(state.Messages) != 4 || !isCompactionSummary(state.Messages[1]) {
		t.Fatalf("unexpected compacted messages: %#v", state.Messages)
	}

	parsed, err := LoadEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 1 || parsed.Events[0].Step.Type != "compaction" {
		t.Fatalf("unexpected compaction events: %#v", parsed.Events)
	}
}

func TestSummarizeToolCall_ExtractsKeyParam(t *testing.T) {
	tests := []struct {
		name string
		call ToolCall
		want string
	}{
		{
			name: "Read with file_path",
			call: ToolCall{Function: ToolCallFunction{Name: "Read", Arguments: `{"file_path":"main.go"}`}},
			want: "Read(main.go)",
		},
		{
			name: "Bash with command",
			call: ToolCall{Function: ToolCallFunction{Name: "Bash", Arguments: `{"command":"go test ./..."}`}},
			want: "Bash(go test ./...)",
		},
		{
			name: "Grep with pattern",
			call: ToolCall{Function: ToolCallFunction{Name: "Grep", Arguments: `{"pattern":"func main"}`}},
			want: "Grep(func main)",
		},
		{
			name: "no arguments",
			call: ToolCall{Function: ToolCallFunction{Name: "Read", Arguments: ""}},
			want: "Read",
		},
		{
			name: "empty name",
			call: ToolCall{Function: ToolCallFunction{Name: "", Arguments: ""}},
			want: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeToolCall(tt.call)
			if got != tt.want {
				t.Errorf("summarizeToolCall() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeCompactedMessage_ToolResultIncludesSize(t *testing.T) {
	msg := Message{
		Role:    "tool",
		Name:    "Read",
		Content: "first line\n" + strings.Repeat("x", 500),
	}
	got := summarizeCompactedMessage(msg)
	if !strings.Contains(got, "first line") {
		t.Error("should include first line of tool result")
	}
	if !strings.Contains(got, "bytes total") {
		t.Error("should include size hint for large results")
	}
}

func TestSummarizeCompactedMessage_ToolResultNoSizeForSmall(t *testing.T) {
	msg := Message{
		Role:    "tool",
		Name:    "Write",
		Content: "Wrote 42 bytes",
	}
	got := summarizeCompactedMessage(msg)
	if strings.Contains(got, "bytes total") {
		t.Error("should not include size hint for small results")
	}
}

func TestAgeToolResults_ShrinksOldToolMessages(t *testing.T) {
	largeContent := strings.Repeat("x", 100*1024) // 100KB
	messages := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "request"},
		{Role: "assistant", Content: "response", ToolCalls: []ToolCall{{ID: "1", Function: ToolCallFunction{Name: "Read"}}}},
		{Role: "tool", Name: "Read", Content: largeContent},
		{Role: "user", Content: "recent request"},
		{Role: "assistant", Content: "recent response"},
	}

	aged := ageToolResults(messages, 2)
	if aged != 1 {
		t.Fatalf("ageToolResults = %d, want 1", aged)
	}

	// The tool result should be much smaller now
	agedBudget := toolOutputBudget("Read") / 4
	if len(messages[3].Content) > agedBudget+200 { // allow marker overhead
		t.Errorf("aged content %d bytes, expected <= %d", len(messages[3].Content), agedBudget+200)
	}
}

func TestAgeToolResults_PreservesRecentMessages(t *testing.T) {
	largeContent := strings.Repeat("x", 100*1024)
	messages := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "tool", Name: "Read", Content: largeContent},  // old — should be aged
		{Role: "user", Content: "middle"},
		{Role: "tool", Name: "Read", Content: largeContent},  // recent — should be preserved
		{Role: "assistant", Content: "reply"},
	}

	aged := ageToolResults(messages, 2)
	// cutoff = 5-2 = 3, so messages[0..2] are in aging range.
	// messages[1] is a large tool result → should be aged.
	if aged != 1 {
		t.Fatalf("ageToolResults = %d, want 1", aged)
	}
	// The old tool result should be shrunk
	agedBudget := toolOutputBudget("Read") / 4
	if len(messages[1].Content) > agedBudget+200 {
		t.Error("old tool result should be aged")
	}
	// The recent tool result should be untouched
	if len(messages[3].Content) != len(largeContent) {
		t.Error("recent tool results should not be aged")
	}
}

func TestApplyCacheBreakpoint(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "prompt", CacheControl: &cacheControl{Type: "ephemeral"}},
		{Role: "user", Content: "hello", CacheControl: &cacheControl{Type: "ephemeral"}},
		{Role: "assistant", Content: "hi"},
		{Role: "tool", Name: "Read", Content: "file"},
	}

	applyCacheBreakpoint(messages)

	// System prompt should still have its breakpoint (we don't touch [0])
	// Wait — we clear from index 1. Let me check the function...
	// The function clears all non-[0] breakpoints, then sets on last message.
	if messages[0].CacheControl == nil {
		t.Error("system prompt should keep cache_control")
	}
	if messages[1].CacheControl != nil {
		t.Error("old breakpoint on user message should be cleared")
	}
	if messages[2].CacheControl != nil {
		t.Error("assistant message should not have breakpoint")
	}
	if messages[3].CacheControl == nil {
		t.Error("last message should have cache breakpoint")
	}
}
