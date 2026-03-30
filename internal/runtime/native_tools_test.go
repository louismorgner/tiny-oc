package runtime

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeEditRejectsEmptyOldString(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeEdit(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Edit", map[string]interface{}{
		"file_path":  "notes.txt",
		"old_string": "",
		"new_string": "updated",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if !strings.Contains(result.Message, "old_string must not be empty") {
		t.Fatalf("unexpected edit failure: %q", result.Message)
	}
}

func TestNativeWriteAndEditPreserveFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho old\n"), 0755); err != nil {
		t.Fatal(err)
	}

	writeResult := nativeWrite(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Write", map[string]interface{}{
		"file_path": "script.sh",
		"content":   "#!/bin/sh\necho new\n",
	}))
	if writeResult.Step.Success == nil || !*writeResult.Step.Success {
		t.Fatalf("write failed: %#v", writeResult)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode after write = %#o, want 0755", info.Mode().Perm())
	}

	editResult := nativeEdit(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Edit", map[string]interface{}{
		"file_path":  "script.sh",
		"old_string": "echo new",
		"new_string": "echo edited",
	}))
	if editResult.Step.Success == nil || !*editResult.Step.Success {
		t.Fatalf("edit failed: %#v", editResult)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode after edit = %#o, want 0755", info.Mode().Perm())
	}
}

func TestNativeGlobReturnsSessionRelativePathsAndSkipsRuntimeDirs(t *testing.T) {
	dir := t.TempDir()
	for _, subdir := range []string{
		filepath.Join(dir, "src"),
		filepath.Join(dir, ".git"),
		filepath.Join(dir, ".toc-native"),
	} {
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".toc-native", "system-prompt.md"), []byte("system\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeGlob(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Glob", map[string]interface{}{
		"pattern": "*.go",
		"path":    "src",
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("glob failed: %#v", result)
	}
	if strings.TrimSpace(result.Message) != "src/main.go" {
		t.Fatalf("glob result = %q", result.Message)
	}

	all := nativeGlob(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Glob", map[string]interface{}{
		"pattern": "**/*",
	}))
	if strings.Contains(all.Message, ".git/config") || strings.Contains(all.Message, ".toc-native/system-prompt.md") {
		t.Fatalf("glob should skip runtime dirs, got %q", all.Message)
	}
}

func TestNativeGrepIncludesHiddenFilesAndSkipsGitDirs(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("TOKEN=git-only\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeGrep(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Grep", map[string]interface{}{
		"pattern": "TOKEN",
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("grep failed: %#v", result)
	}
	if !strings.Contains(result.Message, ".env:1:TOKEN=secret") {
		t.Fatalf("grep missing hidden file match: %q", result.Message)
	}
	if strings.Contains(result.Message, ".git/config") {
		t.Fatalf("grep should skip .git internals: %q", result.Message)
	}
}

func TestNativeBashReportsExitCode(t *testing.T) {
	dir := t.TempDir()
	result := nativeBash(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Bash", map[string]interface{}{
		"command": "printf 'oops'; exit 7",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected bash failure, got %#v", result)
	}
	if result.Step.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.Step.ExitCode)
	}
	if result.Step.DurationMS <= 0 {
		t.Fatalf("expected duration, got %#v", result.Step)
	}
	if !strings.Contains(result.Message, "oops") || !strings.Contains(result.Message, "command exited with code 7") {
		t.Fatalf("unexpected bash failure message: %q", result.Message)
	}
}

func TestNativeBashReportsTimeout(t *testing.T) {
	dir := t.TempDir()
	result := nativeBash(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Bash", map[string]interface{}{
		"command":    "sleep 1",
		"timeout_ms": 10,
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected bash timeout failure, got %#v", result)
	}
	if !result.Step.TimedOut {
		t.Fatalf("expected timeout flag, got %#v", result.Step)
	}
	if result.Step.DurationMS <= 0 {
		t.Fatalf("expected duration, got %#v", result.Step)
	}
	if !strings.Contains(result.Message, "timed out") {
		t.Fatalf("unexpected timeout message: %q", result.Message)
	}
}

func TestNativeBashRejectsEmptyCommand(t *testing.T) {
	dir := t.TempDir()
	result := nativeBash(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Bash", map[string]interface{}{
		"command": "   ",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected bash failure, got %#v", result)
	}
	if !strings.Contains(result.Message, "command is required") {
		t.Fatalf("unexpected empty command message: %q", result.Message)
	}
}

func TestNativeTodoWriteUpdatesState(t *testing.T) {
	dir := t.TempDir()
	state := &State{}

	result := nativeTodoWrite(nativeToolContext{
		SessionDir: dir,
		Agent:      "tester",
		State:      state,
	}, toolCall(t, "TodoWrite", map[string]interface{}{
		"todos": []map[string]interface{}{
			{"content": "Implement TodoWrite", "status": "in_progress", "priority": "high"},
			{"content": "Add tests", "status": "pending", "priority": "medium"},
		},
	}))

	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("expected TodoWrite success, got %#v", result)
	}
	if len(state.Todos) != 2 {
		t.Fatalf("len(state.Todos) = %d, want 2", len(state.Todos))
	}
	if state.Todos[0].Content != "Implement TodoWrite" || state.Todos[1].Status != "pending" {
		t.Fatalf("unexpected todos in state: %#v", state.Todos)
	}
	if !strings.Contains(result.Message, "→ Implement TodoWrite") {
		t.Fatalf("expected in-progress todo in message, got: %q", result.Message)
	}
	if !strings.Contains(result.Message, "○ Add tests") {
		t.Fatalf("expected pending todo in message, got: %q", result.Message)
	}
}

func TestNativeTodoWriteRejectsInvalidStatus(t *testing.T) {
	dir := t.TempDir()
	state := &State{}

	result := nativeTodoWrite(nativeToolContext{
		SessionDir: dir,
		Agent:      "tester",
		State:      state,
	}, toolCall(t, "TodoWrite", map[string]interface{}{
		"todos": []map[string]interface{}{
			{"content": "Bad todo", "status": "doing", "priority": "high"},
		},
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected TodoWrite failure, got %#v", result)
	}
	if !strings.Contains(result.Message, `invalid status "doing"`) {
		t.Fatalf("unexpected error message: %q", result.Message)
	}
	if len(state.Todos) != 0 {
		t.Fatalf("state should be unchanged on failure, got %#v", state.Todos)
	}
}

func TestSummarizeTodosEmpty(t *testing.T) {
	got := summarizeTodos(nil)
	if got != "Cleared todo list." {
		t.Fatalf("expected cleared message, got: %q", got)
	}
}

func TestSummarizeTodosAllStatusIcons(t *testing.T) {
	todos := []TodoItem{
		{Content: "Done task", Status: "completed"},
		{Content: "Active task", Status: "in_progress"},
		{Content: "Waiting task", Status: "pending"},
		{Content: "Dropped task", Status: "cancelled"},
	}
	got := summarizeTodos(todos)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), got)
	}
	expects := []struct {
		icon, text string
	}{
		{"✓", "Done task"},
		{"→", "Active task"},
		{"○", "Waiting task"},
		{"✕", "Dropped task"},
	}
	for i, exp := range expects {
		if !strings.HasPrefix(lines[i], exp.icon+" ") {
			t.Errorf("line %d: expected icon %q, got: %q", i, exp.icon, lines[i])
		}
		if !strings.Contains(lines[i], exp.text) {
			t.Errorf("line %d: expected text %q, got: %q", i, exp.text, lines[i])
		}
	}
}

func TestSummarizeTodosTruncatesMultiByteChars(t *testing.T) {
	// Build a content string of 95 multi-byte runes (each '日' is 3 bytes)
	runes := make([]rune, 95)
	for i := range runes {
		runes[i] = '日'
	}
	content := string(runes)

	todos := []TodoItem{{Content: content, Status: "pending"}}
	got := summarizeTodos(todos)

	// Should be "○ " + 87 runes + "..."
	line := strings.TrimPrefix(got, "○ ")
	resultRunes := []rune(line)
	if len(resultRunes) != 90 { // 87 + 3 for "..."
		t.Fatalf("expected 90 runes after icon, got %d", len(resultRunes))
	}
	if !strings.HasSuffix(line, "...") {
		t.Fatalf("expected trailing ellipsis, got: %q", line)
	}
	// Verify no broken UTF-8: every rune before "..." should be '日'
	for i, r := range resultRunes[:87] {
		if r != '日' {
			t.Fatalf("rune %d is %q, expected '日'", i, r)
		}
	}
}

func TestNativeQuestionNonInteractiveAnswered(t *testing.T) {
	dir := t.TempDir()
	metaDir := MetadataDir(dir, "test-session")
	if err := os.MkdirAll(metaDir, 0700); err != nil {
		t.Fatal(err)
	}

	orig := questionPollTimeout
	questionPollTimeout = 5 * time.Second
	defer func() { questionPollTimeout = orig }()

	// Write the answer file shortly after the question is posted.
	answerPath := filepath.Join(metaDir, "answer.json")
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(answerPath, []byte(`{"answer":"use the left branch"}`), 0600)
	}()

	result := nativeQuestion(nativeToolContext{
		SessionDir: dir,
		Workspace:  dir,
		SessionID:  "test-session",
		Agent:      "tester",
	}, toolCall(t, "Question", map[string]interface{}{
		"question": "Which branch should I use?",
	}))

	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if result.Message != "use the left branch" {
		t.Fatalf("unexpected answer: %q", result.Message)
	}

	// question.json and answer.json should both have been cleaned up
	if _, err := os.Stat(filepath.Join(metaDir, "question.json")); !os.IsNotExist(err) {
		t.Fatal("question.json should be removed after answer is received")
	}
	if _, err := os.Stat(filepath.Join(metaDir, "answer.json")); !os.IsNotExist(err) {
		t.Fatal("answer.json should be removed after answer is received")
	}
}

func TestNativeQuestionNonInteractiveTimeout(t *testing.T) {
	dir := t.TempDir()

	orig := questionPollTimeout
	questionPollTimeout = 600 * time.Millisecond
	defer func() { questionPollTimeout = orig }()

	result := nativeQuestion(nativeToolContext{
		SessionDir: dir,
		Workspace:  dir,
		SessionID:  "test-session",
		Agent:      "tester",
	}, toolCall(t, "Question", map[string]interface{}{
		"question": "Are you there?",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure (timeout), got %#v", result)
	}
	if !strings.Contains(result.Message, "timed out") {
		t.Fatalf("expected timeout message, got %q", result.Message)
	}

	// question.json should have been cleaned up on timeout too
	metaDir := MetadataDir(dir, "test-session")
	if _, err := os.Stat(filepath.Join(metaDir, "question.json")); !os.IsNotExist(err) {
		t.Fatal("question.json should be removed after timeout")
	}
}

func TestNativeQuestionNonInteractiveWritesQuestionFile(t *testing.T) {
	dir := t.TempDir()
	metaDir := MetadataDir(dir, "test-session")

	orig := questionPollTimeout
	questionPollTimeout = 200 * time.Millisecond
	defer func() { questionPollTimeout = orig }()

	// Run in background so we can inspect the question file while it's waiting.
	type result struct{ exec toolExecution }
	ch := make(chan result, 1)
	go func() {
		ch <- result{nativeQuestion(nativeToolContext{
			SessionDir: dir,
			Workspace:  dir,
			SessionID:  "test-session",
			Agent:      "checker",
		}, toolCall(t, "Question", map[string]interface{}{
			"question": "What is the target environment?",
		}))}
	}()

	// Give it a moment to write the file, then check its contents.
	time.Sleep(50 * time.Millisecond)
	data, err := os.ReadFile(filepath.Join(metaDir, "question.json"))
	if err != nil {
		t.Fatalf("question.json not written: %v", err)
	}
	var q struct {
		Question  string `json:"question"`
		SessionID string `json:"session_id"`
		Agent     string `json:"agent"`
	}
	if err := json.Unmarshal(data, &q); err != nil {
		t.Fatalf("question.json parse error: %v", err)
	}
	if q.Question != "What is the target environment?" {
		t.Fatalf("question = %q", q.Question)
	}
	if q.SessionID != "test-session" {
		t.Fatalf("session_id = %q", q.SessionID)
	}
	if q.Agent != "checker" {
		t.Fatalf("agent = %q", q.Agent)
	}
	<-ch // wait for goroutine to finish
}

func toolCall(t *testing.T, name string, args map[string]interface{}) ToolCall {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return ToolCall{
		ID: "call-1",
		Function: ToolCallFunction{
			Name:      name,
			Arguments: string(data),
		},
	}
}
