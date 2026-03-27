package spawn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestIntegrationsSummary(t *testing.T) {
	tests := []struct {
		name string
		cfg  *runtime.SessionConfig
		want string
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: "none",
		},
		{
			name: "no integrations",
			cfg:  &runtime.SessionConfig{},
			want: "none",
		},
		{
			name: "single integration",
			cfg: &runtime.SessionConfig{
				Permissions: agent.Permissions{
					Integrations: map[string]agent.IntegrationPermissions{
						"slack": {
							{Mode: agent.PermOn, Capability: "post:#eng"},
							{Mode: agent.PermAsk, Capability: "post:channels/*"},
							{Mode: agent.PermOn, Capability: "read:channels/*"},
						},
					},
				},
			},
			want: "slack (3 action(s))",
		},
		{
			name: "multiple integrations sorted",
			cfg: &runtime.SessionConfig{
				Permissions: agent.Permissions{
					Integrations: map[string]agent.IntegrationPermissions{
						"exa":   {{Mode: agent.PermOn, Capability: "search"}, {Mode: agent.PermOn, Capability: "find_similar"}, {Mode: agent.PermOn, Capability: "contents"}},
						"slack": {{Mode: agent.PermOn, Capability: "post:#eng"}},
					},
				},
			},
			want: "exa (3 action(s)), slack (1 action(s))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := integrationsSummary(tt.cfg)
			if got != tt.want {
				t.Errorf("integrationsSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDetachedScript_PIDTracking(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test prompt"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Workspace: "/workspace", AgentName: "agent",
		SessionID: "session-123", OutputPath: filepath.Join(dir, "toc-output.txt"),
	}, promptPath)

	if !strings.Contains(script, "echo $$ >") {
		t.Error("script does not write PID file")
	}
	if !strings.Contains(script, filepath.Join(dir, "toc-pid.txt")) {
		t.Error("script does not reference PID path")
	}
}

func TestBuildDetachedScript_ExitCodeCapture(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess", OutputPath: filepath.Join(dir, "toc-output.txt"),
	}, promptPath)

	if !strings.Contains(script, "TOC_EXIT=$?") {
		t.Error("script does not capture exit code")
	}
	if !strings.Contains(script, filepath.Join(dir, "toc-exit-code.txt")) {
		t.Error("script does not reference exit code path")
	}

	// Exit code must be captured before the atomic rename
	exitIdx := strings.Index(script, "TOC_EXIT=$?")
	mvIdx := strings.Index(script, "mv ")
	if exitIdx > mvIdx {
		t.Error("exit code should be captured before atomic rename")
	}
}

func TestBuildDetachedScript_EnvVars(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Model: "sonnet", Workspace: "/my/workspace",
		AgentName: "test-agent", SessionID: "sess-456",
		OutputPath: filepath.Join(dir, "toc-output.txt"),
	}, promptPath)

	if !strings.Contains(script, `export TOC_WORKSPACE="/my/workspace"`) {
		t.Error("script missing TOC_WORKSPACE export")
	}
	if !strings.Contains(script, `export TOC_AGENT="test-agent"`) {
		t.Error("script missing TOC_AGENT export")
	}
	if !strings.Contains(script, `export TOC_SESSION_ID="sess-456"`) {
		t.Error("script missing TOC_SESSION_ID export")
	}
}

func TestBuildDetachedScript_WithModel(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Model: "opus", Workspace: "/ws", AgentName: "agent",
		SessionID: "sess", OutputPath: filepath.Join(dir, "toc-output.txt"),
	}, promptPath)

	if !strings.Contains(script, "--model opus") {
		t.Error("script missing model flag")
	}
}

func TestBuildDetachedScript_WithoutModel(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess", OutputPath: filepath.Join(dir, "toc-output.txt"),
	}, promptPath)

	if strings.Contains(script, "--model") {
		t.Error("script should not contain --model when model is empty")
	}
}

func TestBuildDetachedScript_Resume_UsesResumeFlag(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("resume prompt"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess-789", OutputPath: filepath.Join(dir, "toc-output.txt"),
		Resume: true,
	}, promptPath)

	if !strings.Contains(script, "--resume sess-789") {
		t.Error("resume script should use --resume <session-id> flag")
	}
	if strings.Contains(script, "--continue") {
		t.Error("resume script should not use --continue flag")
	}
	if strings.Contains(script, "--session-id") {
		t.Error("resume script should not use --session-id (--resume already targets the session)")
	}
	if !strings.Contains(script, "echo $$ >") {
		t.Error("resume script should write PID file")
	}
	if !strings.Contains(script, "TOC_EXIT=$?") {
		t.Error("resume script should capture exit code")
	}
}

func TestBuildDetachedScript_NoResume_NoResumeFlag(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess", OutputPath: filepath.Join(dir, "toc-output.txt"),
		Resume: false,
	}, promptPath)

	if strings.Contains(script, "--continue") {
		t.Error("non-resume script should not contain --continue flag")
	}
	if strings.Contains(script, "--resume") {
		t.Error("non-resume script should not contain --resume flag")
	}
	if !strings.Contains(script, "--session-id sess") {
		t.Error("non-resume script should use --session-id")
	}
}

func TestBuildDetachedScript_SessionID(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "abc-123", OutputPath: filepath.Join(dir, "toc-output.txt"),
	}, promptPath)

	if !strings.Contains(script, "--session-id abc-123") {
		t.Error("script should pass --session-id to claude so JSONL files match the toc session ID")
	}
}

func TestBuildDetachedScript_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)
	outputPath := filepath.Join(dir, "toc-output.txt")

	script := runtime.BuildClaudeDetachedScript("toc", runtime.DetachedOptions{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess", OutputPath: outputPath,
	}, promptPath)

	if !strings.Contains(script, "mv") {
		t.Error("script does not do atomic rename")
	}
	if !strings.Contains(script, outputPath+".tmp") {
		t.Error("script does not use .tmp file for output")
	}
}

func TestRestrictNativeSubAgentTools(t *testing.T) {
	cfg := &runtime.SessionConfig{
		Runtime: runtimeinfo.NativeRuntime,
		RuntimeConfig: runtime.SessionRuntimeOptions{
			EnabledTools: []string{"Read", "TodoWrite", "Bash", "SubAgent"},
		},
	}

	restrictNativeSubAgentTools(cfg)

	got := strings.Join(cfg.RuntimeConfig.EnabledTools, ",")
	if got != "Read,Bash" {
		t.Fatalf("EnabledTools = %q, want %q", got, "Read,Bash")
	}
}

func TestLoadOrResolveSessionConfig_PrefersPersistedConfig(t *testing.T) {
	workspace := t.TempDir()
	sessionID := "sess-persisted"
	persisted := &runtime.SessionConfig{
		Agent:   "native-child",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		LLM:     runtime.SessionLLMConfig{Provider: "openrouter"},
	}
	if err := runtime.SaveSessionConfigInWorkspace(workspace, sessionID, persisted); err != nil {
		t.Fatal(err)
	}

	called := false
	loaded, err := loadOrResolveSessionConfig(workspace, sessionID, func() (*agent.AgentConfig, error) {
		called = true
		return &agent.AgentConfig{
			Name:    "native-child",
			Runtime: runtimeinfo.NativeRuntime,
			Model:   "anthropic/claude-sonnet-4",
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("fallback agent config should not be loaded when session config exists")
	}
	if loaded.Model != "openai/gpt-4o-mini" {
		t.Fatalf("loaded model = %q", loaded.Model)
	}
}

func TestNativeDetachedSubSession_FailureThenResume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lifecycle integration test in short mode")
	}

	bin := buildTOCBinary(t)
	t.Setenv("TOC_NATIVE_EXECUTABLE", bin)
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".toc", "agents", "native-child"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".toc", "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".toc", "sessions.yaml"), []byte("sessions: []\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &agent.AgentConfig{
		Name:    "native-child",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(workspace, ".toc", "agents", "native-child")
	if err := os.WriteFile(filepath.Join(agentDir, "oc-agent.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("You are a sub-agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{"message": "simulated non-retryable failure"},
			})
		case 2:
			w.Header().Set("Content-Type", "text/event-stream")
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resume-1",
				"model": "openai/gpt-4o-mini",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]interface{}{
								{
									"index": 0,
									"id":    "call-1",
									"type":  "function",
									"function": map[string]interface{}{
										"name":      "Write",
										"arguments": `{"file_path":"result.txt","content":"resumed ok\n"}`,
									},
								},
							},
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resume-1",
				"model": "openai/gpt-4o-mini",
				"usage": map[string]interface{}{"prompt_tokens": 9, "completion_tokens": 4, "total_tokens": 13},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "tool_calls",
					},
				},
			})
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case 3:
			w.Header().Set("Content-Type", "text/event-stream")
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resume-2",
				"model": "openai/gpt-4o-mini",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "resume complete",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resume-2",
				"model": "openai/gpt-4o-mini",
				"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected request count %d", requests)
		}
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	result, err := SpawnSubSession(cfg, SubSpawnOpts{
		ParentSessionID: "parent-1",
		Prompt:          "fail once then resume",
		WorkspaceDir:    workspace,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, err := waitForSubSessionFinish(workspace, result.SessionID, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.ResolvedStatus(); got != session.StatusCompletedError {
		t.Fatalf("first run status = %q", got)
	}
	if exitCode, err := sess.ReadExitCode(); err != nil || exitCode == 0 {
		t.Fatalf("expected non-zero exit code, got %d err=%v", exitCode, err)
	}

	resumed, err := ResumeSubSession(sess, SubResumeOpts{
		ParentSessionID: "parent-1",
		WorkspaceDir:    workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.SessionID != result.SessionID {
		t.Fatalf("resume returned new session id %q", resumed.SessionID)
	}

	sess, err = waitForSubSessionFinish(workspace, result.SessionID, 10*time.Second)
	if err != nil {
		t.Fatalf("resume finish wait failed after %d requests: %v", requests, err)
	}
	if got := sess.ResolvedStatus(); got != session.StatusCompletedOK {
		t.Fatalf("resumed status = %q", got)
	}
	state, err := runtime.LoadState(sess)
	if err != nil {
		t.Fatal(err)
	}
	if state.ResumeCount != 1 || state.Status != "completed" {
		t.Fatalf("state after resume = %#v", state)
	}
	if state.Usage.InputTokens != 19 || state.Usage.OutputTokens != 9 {
		t.Fatalf("usage after resume = %#v", state.Usage)
	}
	if data, err := os.ReadFile(filepath.Join(sess.WorkspacePath, "result.txt")); err != nil || string(data) != "resumed ok\n" {
		t.Fatalf("result file = %q err=%v", string(data), err)
	}
	if parsed, err := runtime.LoadEventLog(sess); err != nil {
		t.Fatal(err)
	} else if len(parsed.Events) == 0 || parsed.Events[len(parsed.Events)-1].Step.Content != "resume complete" {
		t.Fatalf("events after resume = %#v", parsed.Events)
	}
}

func TestNativeDetachedSubSession_CancelThenResume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lifecycle integration test in short mode")
	}

	bin := buildTOCBinary(t)
	t.Setenv("TOC_NATIVE_EXECUTABLE", bin)
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".toc", "agents", "native-child"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".toc", "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".toc", "sessions.yaml"), []byte("sessions: []\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &agent.AgentConfig{
		Name:    "native-child",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(workspace, ".toc", "agents", "native-child")
	if err := os.WriteFile(filepath.Join(agentDir, "oc-agent.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("You are a cancellable sub-agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		switch requests {
		case 1:
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "cancel-1",
				"model": "openai/gpt-4o-mini",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]interface{}{
								{
									"index": 0,
									"id":    "call-cancel",
									"type":  "function",
									"function": map[string]interface{}{
										"name":      "Bash",
										"arguments": `{"command":"sleep 10"}`,
									},
								},
							},
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "cancel-1",
				"model": "openai/gpt-4o-mini",
				"usage": map[string]interface{}{"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "tool_calls",
					},
				},
			})
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case 2:
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "cancel-2",
				"model": "openai/gpt-4o-mini",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "resumed after cancel",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "cancel-2",
				"model": "openai/gpt-4o-mini",
				"usage": map[string]interface{}{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected request count %d", requests)
		}
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	result, err := SpawnSubSession(cfg, SubSpawnOpts{
		ParentSessionID: "parent-cancel",
		Prompt:          "run a long command",
		WorkspaceDir:    workspace,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, err := waitForNativeActivity(workspace, result.SessionID, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	cancelCmd := exec.Command(bin, "runtime", "cancel", result.SessionID)
	cancelCmd.Dir = workspace
	cancelCmd.Env = append(os.Environ(),
		"TOC_WORKSPACE="+workspace,
		"TOC_AGENT=parent-agent",
		"TOC_SESSION_ID=parent-cancel",
	)
	if output, err := cancelCmd.CombinedOutput(); err != nil {
		t.Fatalf("runtime cancel failed: %v\n%s", err, string(output))
	}

	sess, err = waitForStatus(workspace, result.SessionID, session.StatusCancelled, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	state, err := runtime.LoadState(sess)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != session.StatusCancelled {
		t.Fatalf("state after cancel = %#v", state)
	}
	if state.LastError == "" {
		t.Fatalf("expected cancellation error in state: %#v", state)
	}
	parsed, err := runtime.LoadEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) == 0 || !strings.Contains(parsed.Events[len(parsed.Events)-1].Step.Content, "cancelled") {
		t.Fatalf("events after cancel = %#v", parsed.Events)
	}

	resumed, err := ResumeSubSession(sess, SubResumeOpts{
		ParentSessionID: "parent-cancel",
		WorkspaceDir:    workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.SessionID != result.SessionID {
		t.Fatalf("resume returned new session id %q", resumed.SessionID)
	}

	sess, err = waitForSubSessionFinish(workspace, result.SessionID, 10*time.Second)
	if err != nil {
		t.Fatalf("resume finish wait failed after %d requests: %v", requests, err)
	}
	if got := sess.ResolvedStatus(); got != session.StatusCompletedOK {
		t.Fatalf("resumed status = %q", got)
	}
	state, err = runtime.LoadState(sess)
	if err != nil {
		t.Fatal(err)
	}
	if state.ResumeCount != 1 || state.Status != "completed" {
		t.Fatalf("state after resumed cancel = %#v", state)
	}
	if state.Usage.InputTokens != 15 || state.Usage.OutputTokens != 7 {
		t.Fatalf("usage after resumed cancel = %#v", state.Usage)
	}
}

func buildTOCBinary(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "toc-test")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(output))
	}
	return bin
}

func writeSSEChunk(t *testing.T, w http.ResponseWriter, payload map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
		t.Fatal(err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func waitForSubSessionFinish(workspace, sessionID string, timeout time.Duration) (*session.Session, error) {
	deadline := time.Now().Add(timeout)
	var lastStatus string
	var lastState string
	var lastFiles []string
	var lastOutput string
	for time.Now().Before(deadline) {
		sess, err := session.FindByIDInWorkspace(workspace, sessionID)
		if err == nil {
			status := sess.ResolvedStatus()
			lastStatus = status
			if state, err := runtime.LoadState(sess); err == nil {
				lastState = state.Status
			}
			lastFiles = lastFiles[:0]
			for _, name := range []string{"toc-output.txt", "toc-output.txt.tmp", "toc-exit-code.txt", "toc-pid.txt", "toc-cancelled.txt"} {
				if _, err := os.Stat(filepath.Join(sess.WorkspacePath, name)); err == nil {
					lastFiles = append(lastFiles, name)
				}
			}
			if data, err := os.ReadFile(filepath.Join(sess.WorkspacePath, "toc-output.txt.tmp")); err == nil {
				lastOutput = string(data)
			}
			if status == session.StatusCompletedOK || status == session.StatusCompletedError || status == session.StatusCancelled {
				return sess, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for session %s; last status=%q state=%q files=%v output=%q", sessionID, lastStatus, lastState, lastFiles, lastOutput)
}

func waitForNativeActivity(workspace, sessionID string, timeout time.Duration) (*session.Session, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sess, err := session.FindByIDInWorkspace(workspace, sessionID)
		if err == nil {
			if _, err := sess.ReadPID(); err == nil {
				if _, err := runtime.LoadState(sess); err == nil {
					return sess, nil
				}
				if parsed, err := runtime.LoadEventLog(sess); err == nil && len(parsed.Events) > 0 {
					return sess, nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, os.ErrDeadlineExceeded
}

func waitForPID(workspace, sessionID string, timeout time.Duration) (*session.Session, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sess, err := session.FindByIDInWorkspace(workspace, sessionID)
		if err == nil {
			if _, err := sess.ReadPID(); err == nil {
				return sess, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, os.ErrDeadlineExceeded
}

func waitForStatus(workspace, sessionID, want string, timeout time.Duration) (*session.Session, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sess, err := session.FindByIDInWorkspace(workspace, sessionID)
		if err == nil && sess.ResolvedStatus() == want {
			return sess, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, os.ErrDeadlineExceeded
}
