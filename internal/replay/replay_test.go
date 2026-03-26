package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestParseSessionJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []map[string]interface{}{
		{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "thinking", "thinking": "I need to read the file first to understand the structure"},
				},
			},
		},
		{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "tool_use", "name": "Read", "input": map[string]string{"file_path": "main.go"}},
				},
			},
		},
		{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "text", "text": "Here is what I found."},
					{"type": "tool_use", "name": "Edit", "input": map[string]string{
						"file_path":  "main.go",
						"old_string": "func old() {}",
						"new_string": "func new() {\n\treturn\n}",
					}},
				},
			},
		},
		{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "tool_use", "name": "Bash", "input": map[string]string{"command": "go build ./..."}},
				},
			},
		},
		{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "tool_use", "name": "Skill", "input": map[string]string{"skill": "open-source-cto"}},
				},
			},
		},
		{
			"type": "user",
			"message": map[string]interface{}{
				"role":    "user",
				"content": "some user message",
			},
		},
	}

	var content []byte
	for _, line := range lines {
		b, _ := json.Marshal(line)
		content = append(content, b...)
		content = append(content, '\n')
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	provider, err := runtime.Get(runtime.DefaultRuntime)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := provider.ParseSessionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	steps := parsed.Steps

	if len(steps) != 7 {
		t.Fatalf("got %d steps, want 7", len(steps))
	}

	// thinking
	if steps[0].Type != "thinking" {
		t.Errorf("step 0 type = %q, want thinking", steps[0].Type)
	}

	// Read
	if steps[1].Tool != "Read" || steps[1].Path != "main.go" {
		t.Errorf("step 1 = %+v, want Read main.go", steps[1])
	}

	// text
	if steps[2].Type != "text" {
		t.Errorf("step 2 type = %q, want text", steps[2].Type)
	}

	// Edit
	if steps[3].Tool != "Edit" || steps[3].Path != "main.go" {
		t.Errorf("step 3 = %+v, want Edit main.go", steps[3])
	}
	if steps[3].Added != 3 || steps[3].Removed != 1 {
		t.Errorf("step 3 added=%d removed=%d, want added=3 removed=1", steps[3].Added, steps[3].Removed)
	}

	// Bash
	if steps[4].Tool != "Bash" || steps[4].Command != "go build ./..." {
		t.Errorf("step 4 = %+v, want Bash 'go build ./...'", steps[4])
	}

	// Skill
	if steps[5].Type != "skill" || steps[5].Skill != "open-source-cto" {
		t.Errorf("step 5 = %+v, want skill open-source-cto", steps[5])
	}

	// User message
	if steps[6].Type != "user" || steps[6].Content != "some user message" {
		t.Errorf("step 6 = %+v, want user 'some user message'", steps[6])
	}
}

func TestParseSessionJSONL_MissingFile(t *testing.T) {
	provider, err := runtime.Get(runtime.DefaultRuntime)
	if err != nil {
		t.Fatal(err)
	}

	_, err = provider.ParseSessionLog("/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCollectFilesChanged(t *testing.T) {
	steps := []runtime.Step{
		{Type: "tool", Tool: "Read", Path: "a.go"},
		{Type: "tool", Tool: "Edit", Path: "a.go"},
		{Type: "tool", Tool: "Write", Path: "b.go"},
		{Type: "tool", Tool: "Edit", Path: "a.go"}, // duplicate
	}
	files := collectFilesChanged(steps)
	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
}

func TestParseJSONLLine(t *testing.T) {
	provider, err := runtime.Get(runtime.DefaultRuntime)
	if err != nil {
		t.Fatal(err)
	}

	// Assistant message with a tool call
	line, _ := json.Marshal(map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "tool_use", "name": "Read", "input": map[string]string{"file_path": "main.go"}},
			},
		},
	})
	events := provider.ParseSessionLogLineEvents(line)
	if len(events) != 1 || events[0].Step.Tool != "Read" || events[0].Step.Path != "main.go" {
		t.Errorf("ParseJSONLLineEvents(assistant) = %+v, want [Read main.go]", events)
	}

	// User message — should return a user step
	userLine, _ := json.Marshal(map[string]interface{}{
		"type":    "user",
		"message": map[string]interface{}{"role": "user", "content": "hello"},
	})
	userEvents := provider.ParseSessionLogLineEvents(userLine)
	if len(userEvents) != 1 || userEvents[0].Step.Type != "user" || userEvents[0].Step.Content != "hello" {
		t.Errorf("ParseJSONLLineEvents(user) = %+v, want [user 'hello']", userEvents)
	}

	// Invalid JSON — should return nil
	if events := provider.ParseSessionLogLineEvents([]byte("not json")); events != nil {
		t.Errorf("ParseJSONLLineEvents(invalid) = %+v, want nil", events)
	}

	// Assistant message with thinking
	thinkLine, _ := json.Marshal(map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "thinking", "thinking": "Let me think about this"},
			},
		},
	})
	events = provider.ParseSessionLogLineEvents(thinkLine)
	if len(events) != 1 || events[0].Step.Type != "thinking" {
		t.Errorf("ParseJSONLLineEvents(thinking) = %+v, want [thinking]", events)
	}
}

func TestTruncateThinking(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 100, "short"},
		{"a very long thinking block that should be truncated at some point", 30, "a very long thinking block ..."},
		{"line one\nline two\nline three", 100, "line one line two line three"},
	}
	for _, tt := range tests {
		got := runtime.TruncateThinking(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("TruncateThinking(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestForSession_IncludesNativeStateMetadata(t *testing.T) {
	metaDir := t.TempDir()
	sess := &session.Session{
		ID:            "sess-native",
		Agent:         "native-agent",
		Runtime:       runtimeinfo.NativeRuntime,
		MetadataDir:   metaDir,
		WorkspacePath: t.TempDir(),
		Status:        session.StatusCompletedError,
	}
	if err := runtime.AppendEvent(sess, runtime.Event{Step: runtime.Step{Type: "error", Content: "failed upstream"}}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(sess, &runtime.State{
		Runtime:     runtimeinfo.NativeRuntime,
		SessionID:   sess.ID,
		Agent:       sess.Agent,
		Model:       "openai/gpt-4o-mini",
		ResumeCount: 2,
		LastError:   "failed upstream",
		Usage: runtime.TokenUsageSnapshot{
			InputTokens:  12,
			OutputTokens: 8,
		},
	}); err != nil {
		t.Fatal(err)
	}

	r, err := ForSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	if r.Runtime != runtimeinfo.NativeRuntime || r.Model != "openai/gpt-4o-mini" {
		t.Fatalf("runtime/model = %q/%q", r.Runtime, r.Model)
	}
	if r.Status != session.StatusCompletedError {
		t.Fatalf("status = %q", r.Status)
	}
	if r.ResumeCount != 2 || r.LastError != "failed upstream" {
		t.Fatalf("resume_count/last_error = %d/%q", r.ResumeCount, r.LastError)
	}
	if r.Tokens.InputTokens != 12 || r.Tokens.OutputTokens != 8 {
		t.Fatalf("tokens = %#v", r.Tokens)
	}
}
