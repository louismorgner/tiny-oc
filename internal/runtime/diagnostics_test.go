package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestExtractPanicInfo(t *testing.T) {
	text := "stderr before\npanic: runtime error: index out of range\n\ngoroutine 1 [running]:\nmain.main()\n"
	message, stack := ExtractPanicInfo(text)
	if message != "runtime error: index out of range" {
		t.Fatalf("message = %q", message)
	}
	if !strings.Contains(stack, "goroutine 1 [running]:") {
		t.Fatalf("stack = %q", stack)
	}
}

func TestPreserveCrashInfoFromZombieArtifacts(t *testing.T) {
	metaDir := t.TempDir()
	workDir := t.TempDir()
	sess := &session.Session{
		ID:              "sess-zombie",
		Agent:           "native-agent",
		Runtime:         runtimeinfo.NativeRuntime,
		MetadataDir:     metaDir,
		WorkspacePath:   workDir,
		Status:          session.StatusActive,
		ParentSessionID: "parent-1",
	}

	if err := SaveState(sess, &State{
		Runtime:    runtimeinfo.NativeRuntime,
		SessionID:  sess.ID,
		Agent:      sess.Agent,
		Model:      "openai/gpt-4o-mini",
		Workspace:  t.TempDir(),
		SessionDir: workDir,
		Status:     "running",
		PendingTurn: &TurnCheckpoint{
			Phase: "executing_tools",
			ToolCalls: []ToolCall{
				{ID: "call-1", Function: ToolCallFunction{Name: "Edit"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "toc-pid.txt"), []byte("999999"), 0644); err != nil {
		t.Fatal(err)
	}
	panicText := "panic: runtime error: index out of range [3] with length 2\n\ngoroutine 1 [running]:\nmain.processToolResults()\n"
	if err := os.WriteFile(filepath.Join(workDir, "toc-output.txt.tmp"), []byte(panicText), 0644); err != nil {
		t.Fatal(err)
	}

	if err := PreserveCrashInfo(sess); err != nil {
		t.Fatal(err)
	}

	state, err := LoadState(sess)
	if err != nil {
		t.Fatal(err)
	}
	if state.CrashInfo == nil {
		t.Fatal("expected crash info to be preserved")
	}
	if state.CrashInfo.PanicMessage != "runtime error: index out of range [3] with length 2" {
		t.Fatalf("panic message = %q", state.CrashInfo.PanicMessage)
	}
	if state.CrashInfo.LastToolCall != "Edit" {
		t.Fatalf("last tool call = %q", state.CrashInfo.LastToolCall)
	}
	if state.CrashInfo.CrashTime.IsZero() {
		t.Fatal("expected crash time to be set")
	}
	if !strings.Contains(state.CrashInfo.StackTrace, "main.processToolResults") {
		t.Fatalf("stack trace = %q", state.CrashInfo.StackTrace)
	}
	if state.Status != "crashed" {
		t.Fatalf("status = %q", state.Status)
	}
}

func TestRunNativeSession_PanicPersistsCrashEvent(t *testing.T) {
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a coding agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-native-panic", &SessionConfig{
		Agent:   "native-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	server := newPanicResponseServer(t)
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic from malformed runtime response")
		}

		sess := &session.Session{
			ID:          "sess-native-panic",
			Runtime:     runtimeinfo.NativeRuntime,
			MetadataDir: MetadataDir(metaWorkspace, "sess-native-panic"),
		}
		state, err := LoadState(sess)
		if err != nil {
			t.Fatal(err)
		}
		if state.CrashInfo == nil || state.CrashInfo.PanicMessage == "" {
			t.Fatalf("expected crash info in state, got %#v", state.CrashInfo)
		}
		if state.CrashInfo.CrashTime.Before(time.Now().Add(-time.Minute)) {
			t.Fatalf("unexpected crash time: %v", state.CrashInfo.CrashTime)
		}

		parsed, err := LoadEventLog(sess)
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed.Events) == 0 {
			t.Fatal("expected crash event")
		}
		last := parsed.Events[len(parsed.Events)-1]
		if last.Step.Type != "crash" {
			t.Fatalf("last event = %#v", last.Step)
		}
		if last.Step.StackTrace == "" {
			t.Fatalf("expected crash stack trace, got %#v", last.Step)
		}
	}()

	_ = RunNativeSession(NativeRunOptions{
		Mode:      "detached",
		Dir:       workDir,
		SessionID: "sess-native-panic",
		Agent:     "native-agent",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
		Prompt:    "Trigger a malformed response",
	}, strings.NewReader(""), os.Stdout)
}

func newPanicResponseServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload := map[string]interface{}{
			"id":    "resp-panic",
			"model": "openai/gpt-4o-mini",
			"usage": map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
			"choices": []map[string]interface{}{
				{
					"index": -1,
					"delta": map[string]interface{}{
						"role":    "assistant",
						"content": "boom",
					},
				},
			},
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
			t.Fatal(err)
		}
	}))
}
