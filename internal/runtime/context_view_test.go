package runtime

import (
	"strings"
	"testing"
)

func TestWorkingSet_UpdateFromToolCall(t *testing.T) {
	ws := &WorkingSet{}

	ws.UpdateFromToolCall("Read", `{"file_path":"main.go"}`)
	ws.UpdateFromToolCall("Read", `{"file_path":"util.go"}`)
	ws.UpdateFromToolCall("Read", `{"file_path":"main.go"}`) // duplicate
	ws.UpdateFromToolCall("Edit", `{"file_path":"main.go"}`)
	ws.UpdateFromToolCall("Write", `{"file_path":"new.go"}`)
	ws.UpdateFromToolCall("Bash", `{"command":"go test ./..."}`)
	ws.UpdateFromToolCall("Bash", `{"command":"go build"}`)

	if len(ws.FilesRead) != 2 {
		t.Errorf("FilesRead = %d, want 2 (no duplicates)", len(ws.FilesRead))
	}
	if len(ws.FilesEdited) != 1 || ws.FilesEdited[0] != "main.go" {
		t.Errorf("FilesEdited = %v, want [main.go]", ws.FilesEdited)
	}
	if len(ws.FilesWritten) != 1 || ws.FilesWritten[0] != "new.go" {
		t.Errorf("FilesWritten = %v, want [new.go]", ws.FilesWritten)
	}
	if len(ws.RecentBash) != 2 {
		t.Errorf("RecentBash = %d, want 2", len(ws.RecentBash))
	}
}

func TestWorkingSet_UpdateFromToolCall_EmptyArgs(t *testing.T) {
	ws := &WorkingSet{}
	ws.UpdateFromToolCall("Read", "")
	ws.UpdateFromToolCall("Bash", `{}`)
	if len(ws.FilesRead) != 0 {
		t.Error("should not add entries for empty args")
	}
}

func TestWorkingSet_Summary(t *testing.T) {
	ws := &WorkingSet{
		FilesEdited:  []string{"main.go", "util.go"},
		FilesWritten: []string{"new.go"},
		FilesRead:    []string{"config.go"},
		RecentBash:   []string{"go test ./...", "go build"},
	}
	summary := ws.Summary()
	if !strings.Contains(summary, "Edited:") {
		t.Error("summary should include edited files")
	}
	if !strings.Contains(summary, "main.go") {
		t.Error("summary should mention main.go")
	}
	if !strings.Contains(summary, "Recent commands:") {
		t.Error("summary should include recent commands")
	}
}

func TestWorkingSet_SummaryEmpty(t *testing.T) {
	ws := &WorkingSet{}
	if ws.Summary() != "" {
		t.Error("empty working set should produce empty summary")
	}
	var nilWS *WorkingSet
	if nilWS.Summary() != "" {
		t.Error("nil working set should produce empty summary")
	}
}

func TestWorkingSet_SummaryTruncatesLongLists(t *testing.T) {
	ws := &WorkingSet{}
	for i := 0; i < 20; i++ {
		ws.FilesRead = append(ws.FilesRead, "file"+string(rune('a'+i))+".go")
	}
	summary := ws.Summary()
	// Should only show last 10 reads
	if strings.Contains(summary, "filea.go") {
		t.Error("should not include oldest reads when list is long")
	}
}

func TestBuildContextDiagnostics(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "tool", Name: "Read", Content: strings.Repeat("x", 1000)},
	}
	budgeter := &ContextBudgeter{
		ContextWindow:  128000,
		MaxOutput:      16384,
		ReservedBuffer: 4096,
	}

	diag := BuildContextDiagnostics(messages, budgeter)
	if diag.EstimatedInputTokens <= 0 {
		t.Error("should estimate positive token count")
	}
	if diag.InputBudget != budgeter.InputBudget() {
		t.Errorf("InputBudget = %d, want %d", diag.InputBudget, budgeter.InputBudget())
	}
	if diag.MessageCount != 4 {
		t.Errorf("MessageCount = %d, want 4", diag.MessageCount)
	}
	if diag.BudgetDecision != BudgetContinue {
		t.Errorf("BudgetDecision = %q, want continue", diag.BudgetDecision)
	}
	if len(diag.TopContributors) == 0 {
		t.Error("should have top contributors")
	}

	// Check contributor categories
	found := map[string]bool{}
	for _, c := range diag.TopContributors {
		found[c.Label] = true
	}
	if !found["system"] {
		t.Error("should have system contributor")
	}
	if !found["tool:Read"] {
		t.Error("should have tool:Read contributor")
	}
}

func TestBuildContextView_InjectsWorkingSet(t *testing.T) {
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
		WorkingSet: &WorkingSet{
			FilesEdited: []string{"main.go"},
		},
	}

	view := BuildContextView(state)
	// Should have: system + working set + user + assistant = 4
	if len(view) != 4 {
		t.Fatalf("len(view) = %d, want 4", len(view))
	}
	if !strings.Contains(view[1].Content, "[toc-working-set]") {
		t.Error("expected working set injection after system prompt")
	}
	if !strings.Contains(view[1].Content, "main.go") {
		t.Error("working set should mention edited file")
	}
}

func TestTodoSummary(t *testing.T) {
	summary := todoSummary([]TodoItem{
		{Content: "Implement todo persistence", Status: "in_progress", Priority: "high"},
		{Content: "Add tests", Status: "pending", Priority: "medium"},
	})
	if !strings.Contains(summary, "[toc-todos]") {
		t.Fatal("todo summary should include toc-todos header")
	}
	if !strings.Contains(summary, "in_progress [high] Implement todo persistence") {
		t.Fatalf("todo summary missing first todo: %q", summary)
	}
	if !strings.Contains(summary, "pending [medium] Add tests") {
		t.Fatalf("todo summary missing second todo: %q", summary)
	}
}

func TestBuildContextView_InjectsTodosBeforeWorkingSet(t *testing.T) {
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "hello"},
		},
		Todos: []TodoItem{
			{Content: "Implement TodoWrite", Status: "in_progress", Priority: "high"},
		},
		WorkingSet: &WorkingSet{
			FilesEdited: []string{"main.go"},
		},
	}

	view := BuildContextView(state)
	if len(view) != 4 {
		t.Fatalf("len(view) = %d, want 4", len(view))
	}
	if !strings.Contains(view[1].Content, "[toc-todos]") {
		t.Fatalf("expected todo injection at view[1], got %q", view[1].Content)
	}
	if !strings.Contains(view[2].Content, "[toc-working-set]") {
		t.Fatalf("expected working set injection at view[2], got %q", view[2].Content)
	}
}

func TestBuildContextView_SkipsNilTodos(t *testing.T) {
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "hello"},
		},
		Todos: nil,
		WorkingSet: &WorkingSet{
			FilesEdited: []string{"main.go"},
		},
	}

	view := BuildContextView(state)
	if len(view) != 3 {
		t.Fatalf("len(view) = %d, want 3", len(view))
	}
	if strings.Contains(view[1].Content, "[toc-todos]") {
		t.Fatalf("did not expect todo injection at view[1], got %q", view[1].Content)
	}
	if !strings.Contains(view[1].Content, "[toc-working-set]") {
		t.Fatalf("expected working set injection at view[1], got %q", view[1].Content)
	}
}

func TestBuildContextView_NoWorkingSet(t *testing.T) {
	state := &State{
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "hello"},
		},
	}

	view := BuildContextView(state)
	if len(view) != 2 {
		t.Fatalf("len(view) = %d, want 2 (no working set injection)", len(view))
	}
}

func TestBuildContextView_NilState(t *testing.T) {
	view := BuildContextView(nil)
	if view != nil {
		t.Error("nil state should return nil view")
	}
}

func TestAppendUnique(t *testing.T) {
	slice := []string{"a", "b"}
	slice = appendUnique(slice, "b", 10) // duplicate
	if len(slice) != 2 {
		t.Error("should not add duplicate")
	}
	slice = appendUnique(slice, "c", 10)
	if len(slice) != 3 {
		t.Error("should add new unique value")
	}
	// Test cap
	for i := 0; i < 20; i++ {
		slice = appendUnique(slice, string(rune('d'+i)), 5)
	}
	if len(slice) > 5 {
		t.Errorf("should cap at 5, got %d", len(slice))
	}
}
