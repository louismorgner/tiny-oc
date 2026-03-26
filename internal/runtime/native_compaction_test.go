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
