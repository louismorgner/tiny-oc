package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestCompactMessagesStructured_PreservesSystemAndRecentTail(t *testing.T) {
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "first request"},
			{Role: "assistant", Content: "first answer"},
			{Role: "tool", Name: "Read", Content: "file contents"},
			{Role: "user", Content: "second request"},
			{Role: "assistant", Content: "second answer"},
		},
	}

	compacted, compactedCount, _ := compactMessagesStructured(state, 2, 512, nil)
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

func TestResolveCompactionModel(t *testing.T) {
	state := &State{Model: "openai/gpt-4o"}

	if got := resolveCompactionModel(state, nil); got != "openai/gpt-4o" {
		t.Fatalf("resolveCompactionModel(nil cfg) = %q", got)
	}
	if got := resolveCompactionModel(state, &SessionConfig{Model: "openai/gpt-4o-mini"}); got != "openai/gpt-4o-mini" {
		t.Fatalf("resolveCompactionModel(primary fallback) = %q", got)
	}
	if got := resolveCompactionModel(state, &SessionConfig{
		Model:      "openai/gpt-4o-mini",
		SmallModel: "anthropic/claude-sonnet-4",
	}); got != "anthropic/claude-sonnet-4" {
		t.Fatalf("resolveCompactionModel(small model) = %q", got)
	}
}

func TestMaybeManageContext_UsesSmallModelForCompaction(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-small-model",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}

	state := &State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Model:     "openai/gpt-4o",
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: strings.Repeat("a", 2500)},
			{Role: "assistant", Content: strings.Repeat("b", 2500)},
			{Role: "tool", Name: "Read", Content: strings.Repeat("c", 2500)},
			{Role: "user", Content: "keep"},
			{Role: "assistant", Content: "recent"},
		},
	}
	cfg := &SessionConfig{
		Model:      "openai/gpt-4o",
		SmallModel: "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			CompactionKeepRecent: 2,
		},
	}
	profile := runtimeinfo.NativeModelProfile{
		ContextWindow:   1800,
		MaxOutputTokens: 512,
		ReservedBuffer:  256,
	}

	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "resp-compaction",
			"model": req.Model,
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"goal":"keep context","completed_work":["summarized earlier work"],"next_steps":["continue from recent messages"]}`,
					},
					"finish_reason": "stop",
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &openRouterClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: server.Client(),
	}

	changed, err := maybeManageContext(state, sess, cfg, profile, client)
	if err != nil {
		t.Fatalf("maybeManageContext() error = %v", err)
	}
	if !changed {
		t.Fatal("expected context compaction to change state")
	}
	if gotModel != cfg.SmallModel {
		t.Fatalf("compaction model = %q, want %q", gotModel, cfg.SmallModel)
	}
	if len(state.Messages) < 2 || !isContinuationArtifact(state.Messages[1]) {
		t.Fatalf("expected continuation artifact after compaction, got %#v", state.Messages)
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
	// Continuation artifacts should also be recognized
	cont := Message{Role: "user", Content: "[toc-continuation]\nstructured data"}
	if !isCompactionSummary(cont) {
		t.Fatal("should recognize continuation artifact as compaction summary")
	}
	// Regular messages should not match.
	regular := Message{Role: "user", Content: "hello world"}
	if isCompactionSummary(regular) {
		t.Fatal("regular user message should not be a compaction summary")
	}
}

func TestBudgetFail_EmergencyCompaction(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-fail",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}

	// Tiny context window to force BudgetFail immediately.
	// Budget = 2048 - 512 - 256 = 1280 tokens. Fail threshold = 98% = 1254 tokens.
	// With cl100k_base, repeated chars ≈ 8 bytes/token, so 3 × 8000 bytes ≈ 3000 tokens
	// + overhead, well above 1254.
	profile := runtimeinfo.NativeModelProfile{
		ContextWindow:   2048,
		MaxOutputTokens: 512,
		ReservedBuffer:  256,
	}

	state := &State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: strings.Repeat("a", 8000)},
			{Role: "assistant", Content: strings.Repeat("b", 8000)},
			{Role: "tool", Name: "Edit", Content: strings.Repeat("c", 8000)}, // prune-protected
			{Role: "user", Content: "keep"},
			{Role: "assistant", Content: "recent"},
		},
	}

	changed, err := manageContextWithBudget(state, sess, nil, 2, 6000, profile, nil)

	// Emergency compaction should have fired. If it managed to compact below
	// the fail threshold, changed=true and no error. If still over, we get an error.
	if err != nil {
		// Error means compaction couldn't bring it under budget — acceptable
		if !strings.Contains(err.Error(), "context exceeds model budget") {
			t.Fatalf("unexpected error: %v", err)
		}
	} else if !changed {
		t.Fatal("expected emergency compaction to make changes")
	}

	// Either way, compaction count should be incremented if compaction happened
	if state.CompactionCount > 0 {
		if state.LastCompactedAt.IsZero() {
			t.Error("LastCompactedAt should be set after emergency compaction")
		}
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
		{Role: "tool", Name: "Read", Content: largeContent}, // old — should be aged
		{Role: "user", Content: "middle"},
		{Role: "tool", Name: "Read", Content: largeContent}, // recent — should be preserved
		{Role: "assistant", Content: "reply"},
	}

	aged := ageToolResults(messages, 2)
	if aged != 1 {
		t.Fatalf("ageToolResults = %d, want 1", aged)
	}
	agedBudget := toolOutputBudget("Read") / 4
	if len(messages[1].Content) > agedBudget+200 {
		t.Error("old tool result should be aged")
	}
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

// --- New tests for token-budget-based context management ---

func TestContextBudgeter_Evaluate(t *testing.T) {
	budgeter := &ContextBudgeter{
		ContextWindow:  128000,
		MaxOutput:      16384,
		ReservedBuffer: 4096,
	}

	budget := budgeter.InputBudget() // 128000 - 16384 - 4096 = 107520

	tests := []struct {
		name   string
		tokens int
		want   BudgetDecision
	}{
		{"low usage", budget / 2, BudgetContinue},
		{"below prune", budget*3/4 - 1, BudgetContinue},
		{"at prune threshold", budget * 3 / 4, BudgetPrune},
		{"between prune and compact", budget * 4 / 5, BudgetPrune},
		{"at compact threshold", budget * 9 / 10, BudgetCompact},
		{"near fail", budget * 97 / 100, BudgetCompact},
		{"at fail threshold", budget * 98 / 100, BudgetFail},
		{"over budget", budget + 1000, BudgetFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := budgeter.Evaluate(tt.tokens)
			if got != tt.want {
				t.Errorf("Evaluate(%d) = %q, want %q (budget=%d, prune=%d, compact=%d, fail=%d)",
					tt.tokens, got, tt.want, budget, budgeter.PruneThreshold(), budgeter.CompactThreshold(), budgeter.FailThreshold())
			}
		})
	}
}

func TestContextBudgeter_InputBudgetMinimum(t *testing.T) {
	budgeter := &ContextBudgeter{
		ContextWindow:  1000,
		MaxOutput:      800,
		ReservedBuffer: 800,
	}
	// Budget would be negative, but should floor at 1024
	if budgeter.InputBudget() != 1024 {
		t.Errorf("InputBudget() = %d, want 1024", budgeter.InputBudget())
	}
}

func TestNewContextBudgeter_FromProfile(t *testing.T) {
	profile := runtimeinfo.NativeModelProfile{
		ContextWindow:   200000,
		MaxOutputTokens: 16384,
		ReservedBuffer:  4096,
	}
	budgeter := NewContextBudgeter(profile)
	if budgeter.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000", budgeter.ContextWindow)
	}
	if budgeter.MaxOutput != 16384 {
		t.Errorf("MaxOutput = %d, want 16384", budgeter.MaxOutput)
	}
}

func TestNewContextBudgeter_Defaults(t *testing.T) {
	profile := runtimeinfo.NativeModelProfile{} // all zeroes
	budgeter := NewContextBudgeter(profile)
	if budgeter.ContextWindow != 128000 {
		t.Errorf("ContextWindow = %d, want 128000", budgeter.ContextWindow)
	}
}

func TestPruneStaleToolOutputs_ProtectsErrors(t *testing.T) {
	// Read aged budget = 48KB/4 = 12KB; use content larger than that
	largeContent := strings.Repeat("y", 50*1024)
	messages := []Message{
		{Role: "system", Content: "prompt"},
		{Role: "tool", Name: "Bash", Content: "error: compilation failed\n" + strings.Repeat("x", 50*1024)},
		{Role: "tool", Name: "Read", Content: largeContent},
		{Role: "user", Content: "recent"},
		{Role: "assistant", Content: "reply"},
	}

	origErrorLen := len(messages[1].Content)
	pruned := pruneStaleToolOutputs(messages, 2)

	// Read output should be pruned, error output should be protected
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	if len(messages[1].Content) != origErrorLen {
		t.Error("error tool output should be protected from pruning")
	}
	if len(messages[2].Content) >= len(largeContent) {
		t.Error("Read tool output should have been pruned")
	}
}

func TestPruneStaleToolOutputs_ProtectsEditAndWrite(t *testing.T) {
	// Read aged budget = 48KB/4 = 12KB; use content larger than that
	largeRead := strings.Repeat("r", 50*1024)
	messages := []Message{
		{Role: "system", Content: "prompt"},
		{Role: "tool", Name: "Edit", Content: strings.Repeat("e", 2000)},
		{Role: "tool", Name: "Write", Content: strings.Repeat("w", 2000)},
		{Role: "tool", Name: "Read", Content: largeRead},
		{Role: "user", Content: "recent"},
		{Role: "assistant", Content: "reply"},
	}

	origEditLen := len(messages[1].Content)
	origWriteLen := len(messages[2].Content)
	pruned := pruneStaleToolOutputs(messages, 2)

	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1 (only Read)", pruned)
	}
	if len(messages[1].Content) != origEditLen {
		t.Error("Edit output should be protected")
	}
	if len(messages[2].Content) != origWriteLen {
		t.Error("Write output should be protected")
	}
}

func TestPruneStaleToolOutputs_SkipsSmallOutputs(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "prompt"},
		{Role: "tool", Name: "Read", Content: "small output"},
		{Role: "user", Content: "recent"},
		{Role: "assistant", Content: "reply"},
	}

	pruned := pruneStaleToolOutputs(messages, 2)
	if pruned != 0 {
		t.Fatalf("pruned = %d, want 0 (output too small)", pruned)
	}
}

func TestLooksLikeError(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"error: something went wrong", true},
		{"FAILED to compile", true},
		{"fatal: not a git repository", true},
		{"panic: runtime error", true},
		{"exit code 1", true},
		{"Successfully wrote file", false},
		{"package main\nfunc main() {}", false},
	}
	for _, tt := range tests {
		if got := looksLikeError(tt.content); got != tt.want {
			t.Errorf("looksLikeError(%q) = %v, want %v", tt.content[:min(30, len(tt.content))], got, tt.want)
		}
	}
}

func TestCompactMessagesStructured_ProducesContinuation(t *testing.T) {
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "Please fix the bug in main.go"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{
				{ID: "1", Function: ToolCallFunction{Name: "Read", Arguments: `{"file_path":"main.go"}`}},
			}},
			{Role: "tool", Name: "Read", Content: "package main\nfunc main() {}"},
			{Role: "assistant", Content: "I found the issue"},
			{Role: "user", Content: "keep this"},
			{Role: "assistant", Content: "latest"},
		},
		WorkingSet: &WorkingSet{
			FilesEdited: []string{"main.go"},
		},
	}

	compacted, count, text := compactMessagesStructured(state, 2, 6000, nil)

	if count != 4 {
		t.Fatalf("compactedCount = %d, want 4", count)
	}
	if len(compacted) != 4 { // system + continuation + 2 recent
		t.Fatalf("len(compacted) = %d, want 4", len(compacted))
	}
	if !isContinuationArtifact(compacted[1]) {
		t.Fatal("expected continuation artifact")
	}
	if !strings.Contains(text, "Goal") {
		t.Error("continuation should include Goal section")
	}
	if !strings.Contains(text, "Working Files") {
		t.Error("continuation should include Working Files section")
	}
	if !strings.Contains(text, "main.go") {
		t.Error("continuation should mention main.go from working set")
	}
	if state.Continuation == nil {
		t.Fatal("state.Continuation should be set after structured compaction")
	}
	if state.Continuation.Goal == "" {
		t.Error("continuation goal should be extracted from first user message")
	}
}

func TestManageContextWithBudget_PrunesBeforeCompaction(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-budget",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}

	// Create a state with large tool outputs that should trigger pruning
	// at 75% of budget. Budget = 128000 - 16384 - 4096 = 107520 tokens
	// 75% = 80640 tokens. With cl100k_base, repeated "x" ≈ 8 bytes/token,
	// so 200KB ≈ 25600 tokens × 4 messages ≈ 102400 tokens > 80640.
	largeContent := strings.Repeat("x", 200*1024) // ~200KB ≈ 25600 tokens each
	state := &State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "request"},
			{Role: "tool", Name: "Read", Content: largeContent},
			{Role: "tool", Name: "Read", Content: largeContent},
			{Role: "tool", Name: "Read", Content: largeContent},
			{Role: "tool", Name: "Read", Content: largeContent},
			{Role: "user", Content: "recent"},
			{Role: "assistant", Content: "reply"},
		},
	}

	profile := runtimeinfo.NativeModelProfile{
		ContextWindow:   128000,
		MaxOutputTokens: 16384,
		ReservedBuffer:  4096,
	}

	changed, err := manageContextWithBudget(state, sess, nil, 2, 6000, profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected context management to make changes")
	}

	// Tool outputs should have been pruned
	for i := 2; i <= 5; i++ {
		if len(state.Messages[i].Content) >= len(largeContent) {
			t.Errorf("message[%d] should have been pruned", i)
		}
	}
}

func TestManageContextWithBudget_CompactsWhenNeeded(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-budget-compact",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}

	// Use a context window where the messages exceed the compact threshold
	// (90% of input budget) but the compacted result fits within the budget.
	// Budget = 32768 - 2048 - 512 = 30208 tokens
	// Compact threshold = 90% of 30208 = 27187 tokens
	// With cl100k_base, repeated single chars ≈ 8 bytes/token,
	// so 4 × 60000 bytes ≈ 4 × 7500 = 30000 tokens + overhead > 27187.
	// After compaction, old messages collapse to a small continuation artifact,
	// system prompt is tiny, and only 2 recent messages remain — fits easily.
	profile := runtimeinfo.NativeModelProfile{
		ContextWindow:   32768,
		MaxOutputTokens: 2048,
		ReservedBuffer:  512,
	}

	state := &State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: strings.Repeat("a", 60000)},
			{Role: "assistant", Content: strings.Repeat("b", 60000)},
			{Role: "tool", Name: "Edit", Content: strings.Repeat("c", 60000)}, // Edit is prune-protected
			{Role: "user", Content: strings.Repeat("d", 60000)},
			{Role: "user", Content: "keep"},
			{Role: "assistant", Content: "recent"},
		},
	}

	changed, err := manageContextWithBudget(state, sess, nil, 2, 6000, profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected compaction")
	}
	if state.CompactionCount != 1 {
		t.Fatalf("CompactionCount = %d, want 1", state.CompactionCount)
	}
	// Should have continuation artifact
	found := false
	for _, msg := range state.Messages {
		if isContinuationArtifact(msg) {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected continuation artifact in compacted messages")
	}
}

func TestIsContinuationArtifact(t *testing.T) {
	yes := Message{Role: "user", Content: "[toc-continuation]\nstructured data"}
	if !isContinuationArtifact(yes) {
		t.Fatal("should recognize continuation artifact")
	}
	no := Message{Role: "user", Content: "regular message"}
	if isContinuationArtifact(no) {
		t.Fatal("should not match regular message")
	}
	// isCompactionSummary should also recognize continuation artifacts
	if !isCompactionSummary(yes) {
		t.Fatal("isCompactionSummary should recognize continuation artifacts")
	}
}

func TestMaybeManageContext_ZeroContextWindowUsesDefaults(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-defaults",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}

	state := &State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "small message"},
			{Role: "assistant", Content: "small reply"},
		},
	}

	// Profile with no context window — budgeter will use defaults (128K).
	// Small messages should not trigger any compaction.
	profile := runtimeinfo.NativeModelProfile{}
	cfg := &SessionConfig{}

	changed, err := maybeManageContext(state, sess, cfg, profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("small messages should not trigger compaction even with zero-value profile")
	}
}

func TestResumeAfterCompaction_WorksWithStructuredContinuation(t *testing.T) {
	// Simulate: compact, then add new messages, then compact again
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "fix the auth bug"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{
				{ID: "1", Function: ToolCallFunction{Name: "Read", Arguments: `{"file_path":"auth.go"}`}},
			}},
			{Role: "tool", Name: "Read", Content: "package auth"},
			{Role: "user", Content: "keep1"},
			{Role: "assistant", Content: "keep2"},
		},
		WorkingSet: &WorkingSet{FilesEdited: []string{"auth.go"}},
	}

	// First compaction
	compacted1, count1, _ := compactMessagesStructured(state, 2, 6000, nil)
	if count1 == 0 {
		t.Fatal("expected first compaction")
	}
	state.Messages = compacted1

	// Add more messages simulating continued work
	state.Messages = append(state.Messages,
		Message{Role: "user", Content: "now fix the tests"},
		Message{Role: "assistant", Content: "", ToolCalls: []ToolCall{
			{ID: "2", Function: ToolCallFunction{Name: "Edit", Arguments: `{"file_path":"auth_test.go"}`}},
		}},
		Message{Role: "tool", Name: "Edit", Content: "ok"},
		Message{Role: "user", Content: "latest"},
		Message{Role: "assistant", Content: "done"},
	)

	// Second compaction
	compacted2, count2, text := compactMessagesStructured(state, 2, 6000, nil)
	if count2 == 0 {
		t.Fatal("expected second compaction")
	}

	// The continuation should reference the original goal
	if !strings.Contains(text, "auth") {
		t.Error("second continuation should reference original auth work")
	}

	// Should still have system + continuation + 2 recent
	if len(compacted2) != 4 {
		t.Fatalf("len(compacted) = %d, want 4", len(compacted2))
	}
}

func TestPruneMarker_UsedForSmallBudgetTools(t *testing.T) {
	// Glob budget = 8KB, aged = 2KB. Write/Edit have 1KB budget → aged = 256
	// which is < 512 so they'd get prune marker. But Write/Edit are protected.
	// Use a tool with small budget to test the marker.
	messages := []Message{
		{Role: "system", Content: "prompt"},
		{Role: "tool", Name: "Glob", Content: strings.Repeat("x", 50*1024)}, // way over Glob budget
		{Role: "user", Content: "recent"},
		{Role: "assistant", Content: "reply"},
	}

	pruned := pruneStaleToolOutputs(messages, 2)
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}

	// Glob budget = 8KB, aged = 2KB > 512, so should use truncateMiddle not marker
	if messages[1].Content == pruneMarker {
		t.Error("Glob aged budget (2KB) should use truncateMiddle, not prune marker")
	}
	if !strings.Contains(messages[1].Content, "truncated") {
		t.Error("should contain truncation marker")
	}
}

func TestStateMigration_V5ToV6(t *testing.T) {
	state := &State{
		Version: 5,
		Messages: []Message{
			{Role: "system", Content: "prompt"},
			{Role: "user", Content: "hello"},
		},
	}
	migrateState(state)

	if state.Version != StateVersion {
		t.Errorf("Version = %d, want %d", state.Version, StateVersion)
	}
	if len(state.Transcript) != 2 {
		t.Fatalf("Transcript should have 2 messages after migration, got %d", len(state.Transcript))
	}
	if state.Transcript[0].Content != "prompt" {
		t.Error("Transcript should match Messages content")
	}
}

func TestStateMigration_AlreadyV6(t *testing.T) {
	state := &State{
		Version: 6,
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
		Transcript: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "old message"},
		},
	}
	migrateState(state)

	// Should not overwrite existing Transcript
	if len(state.Transcript) != 2 {
		t.Fatalf("Transcript should not be overwritten, got %d", len(state.Transcript))
	}
}

func TestTranscriptSurvivesCompaction(t *testing.T) {
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "reply1"},
			{Role: "tool", Name: "Read", Content: strings.Repeat("r", 4000)},
			{Role: "user", Content: "keep"},
			{Role: "assistant", Content: "keep2"},
		},
	}
	// Copy messages to transcript (simulating the runner dual-append)
	state.Transcript = make([]Message, len(state.Messages))
	copy(state.Transcript, state.Messages)

	compacted, count, _ := compactMessagesStructured(state, 2, 6000, nil)
	if count == 0 {
		t.Fatal("expected compaction")
	}
	state.Messages = compacted

	// Messages should be compacted (fewer messages)
	if len(state.Messages) >= 6 {
		t.Errorf("Messages should be compacted, got %d", len(state.Messages))
	}
	// Transcript should be untouched
	if len(state.Transcript) != 6 {
		t.Errorf("Transcript should be preserved (6), got %d", len(state.Transcript))
	}
	if state.Transcript[1].Content != "first" {
		t.Error("Transcript should still have original messages")
	}
}
