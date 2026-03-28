package runtime

import (
	"bytes"
	"encoding/json"
	"io"
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

func TestRunNativeSession_DetachedToolLoop(t *testing.T) {
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()
	agentDir := filepath.Join(metaWorkspace, ".toc", "agents", "native-agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a coding agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "notes.txt"), []byte("hello from file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "memory.md"), []byte("initial memory\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-native-loop", &SessionConfig{
		Agent:   "native-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		Context: []string{"memory.md"},
		OnEnd:   "Write memory.md to say persisted memory",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "openai/gpt-4o-mini" {
			t.Fatalf("model = %q", req.Model)
		}
		if req.Provider == nil || !req.Provider.RequireParameters {
			t.Fatalf("provider preference missing: %#v", req.Provider)
		}

		if !req.Stream {
			t.Fatalf("expected streaming request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch callCount {
		case 1:
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-1",
				"model": req.Model,
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
										"name":      "Read",
										"arguments": `{"file_path":"notes.txt"}`,
									},
								},
							},
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-1",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "tool_calls",
					},
				},
			})
		case 2:
			if len(req.Messages) < 3 {
				t.Fatalf("expected tool result message, got %d messages", len(req.Messages))
			}
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "call-1" {
				t.Fatalf("last message = %#v", last)
			}
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-2",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "Done reading ",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-2",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 11, "completion_tokens": 6, "total_tokens": 17},
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"content": "notes.txt",
						},
						"finish_reason": "stop",
					},
				},
			})
		case 3:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "user" || last.Content != "Write memory.md to say persisted memory" {
				t.Fatalf("unexpected on_end message %#v", last)
			}
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-3",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]interface{}{
								{
									"index": 0,
									"id":    "call-2",
									"type":  "function",
									"function": map[string]interface{}{
										"name":      "Write",
										"arguments": `{"file_path":"memory.md","content":"persisted memory\n"}`,
									},
								},
							},
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-3",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 12, "completion_tokens": 7, "total_tokens": 19},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "tool_calls",
					},
				},
			})
		case 4:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "call-2" {
				t.Fatalf("expected write tool result, got %#v", last)
			}
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-4",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "Persisted memory",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-4",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 13, "completion_tokens": 8, "total_tokens": 21},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
		default:
			t.Fatalf("unexpected request count %d", callCount)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	var stdout bytes.Buffer
	err := RunNativeSession(NativeRunOptions{
		Mode:      "detached",
		Dir:       workDir,
		SessionID: "sess-native-loop",
		Agent:     "native-agent",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
		Prompt:    "Read notes.txt and summarize it.",
	}, bytes.NewBuffer(nil), &stdout)
	if err != nil {
		t.Fatal(err)
	}

	if got := stdout.String(); got != "Done reading notes.txt\nPersisted memory\n" {
		t.Fatalf("stdout = %q", got)
	}

	state, err := LoadStateInWorkspace(metaWorkspace, "sess-native-loop")
	if err != nil {
		t.Fatal(err)
	}
	if state.Runtime != runtimeinfo.NativeRuntime || state.Status != "completed" {
		t.Fatalf("state = %#v", state)
	}
	if state.Usage.InputTokens != 46 || state.Usage.OutputTokens != 26 {
		t.Fatalf("expected usage totals in state, got %#v", state.Usage)
	}
	if len(state.Messages) != 9 {
		t.Fatalf("expected 9 messages, got %d", len(state.Messages))
	}
	syncedMemory, err := os.ReadFile(filepath.Join(agentDir, "memory.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(syncedMemory) != "persisted memory\n" {
		t.Fatalf("synced memory = %q", string(syncedMemory))
	}

	sess := &session.Session{
		ID:          "sess-native-loop",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: MetadataDir(metaWorkspace, "sess-native-loop"),
	}
	parsed, err := LoadEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) < 2 {
		t.Fatalf("expected events, got %#v", parsed.Events)
	}
	if parsed.Events[0].Step.Tool != "Read" {
		t.Fatalf("expected read tool event, got %#v", parsed.Events)
	}
	if parsed.Events[len(parsed.Events)-2].Step.Tool != "Write" {
		t.Fatalf("expected write tool event, got %#v", parsed.Events)
	}
	if parsed.Events[len(parsed.Events)-1].Step.Content != "Persisted memory" {
		t.Fatalf("unexpected final event %#v", parsed.Events[len(parsed.Events)-1])
	}
}

func TestRunNativeSession_UsesTOCNativeBaseURL(t *testing.T) {
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a coding agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-native-base-url", &SessionConfig{
		Agent:   "native-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "openai/gpt-4o-mini" {
			t.Fatalf("model = %q", req.Model)
		}
		if !req.Stream {
			t.Fatalf("expected streaming request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		writeSSEChunk(t, w, map[string]interface{}{
			"id":    "resp-native-base-url-1",
			"model": req.Model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"role":    "assistant",
						"content": "Custom base URL works.",
					},
				},
			},
		})
		writeSSEChunk(t, w, map[string]interface{}{
			"id":    "resp-native-base-url-1",
			"model": req.Model,
			"usage": map[string]interface{}{"prompt_tokens": 9, "completion_tokens": 4, "total_tokens": 13},
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": "stop",
				},
			},
		})
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("TOC_NATIVE_BASE_URL", server.URL)

	var stdout bytes.Buffer
	err := RunNativeSession(NativeRunOptions{
		Mode:      "detached",
		Dir:       workDir,
		SessionID: "sess-native-base-url",
		Agent:     "native-agent",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
		Prompt:    "Say the custom base URL is working.",
	}, bytes.NewBuffer(nil), &stdout)
	if err != nil {
		t.Fatal(err)
	}

	if got := stdout.String(); got != "Custom base URL works.\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunNativeSession_ContinuesAfterSubAgentNotification(t *testing.T) {
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(metaWorkspace, ".toc", "agents", "native-parent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "oc-agent.yaml"), []byte("runtime: toc-native\nname: native-parent\nmodel: openai/gpt-4o-mini\npermissions:\n  sub-agents:\n    cto: on\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are an orchestrator.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-native-parent", &SessionConfig{
		Agent:   "native-parent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		switch callCount {
		case 1:
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-sub-1",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]interface{}{
								{
									"index": 0,
									"id":    "call-sub-1",
									"type":  "function",
									"function": map[string]interface{}{
										"name":      "SubAgent",
										"arguments": `{"action":"spawn","agent":"cto","prompt":"Review the architecture and report back"}`,
									},
								},
							},
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-sub-1",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "tool_calls",
					},
				},
			})
		case 2:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "call-sub-1" {
				t.Fatalf("expected sub-agent tool result, got %#v", last)
			}
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-sub-2",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "Spawned cto and waiting for completion.",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-sub-2",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
		case 3:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "user" || !strings.Contains(last.Content, "Sub-agent completion update.") {
				t.Fatalf("expected sub-agent completion notification, got %#v", last)
			}
			if !strings.Contains(last.Content, "Agent: cto") || !strings.Contains(last.Content, "child delivered result") {
				t.Fatalf("unexpected completion prompt %q", last.Content)
			}
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-sub-3",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "Integrated child result.",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-sub-3",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 14, "completion_tokens": 6, "total_tokens": 20},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
		default:
			t.Fatalf("unexpected request count %d", callCount)
		}

		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	spawnFunc := func(agentName, prompt, workspace, parentSessionID string) (*SubAgentSpawnResult, error) {
		childWorkDir := t.TempDir()
		if err := session.AddInWorkspace(workspace, session.Session{
			ID:              "child-native-1",
			Agent:           agentName,
			Runtime:         DefaultRuntime,
			MetadataDir:     MetadataDir(workspace, "child-native-1"),
			CreatedAt:       time.Now(),
			WorkspacePath:   childWorkDir,
			Status:          session.StatusActive,
			ParentSessionID: parentSessionID,
			Prompt:          prompt,
		}); err != nil {
			return nil, err
		}

		go func() {
			time.Sleep(200 * time.Millisecond)
			_ = os.WriteFile(filepath.Join(childWorkDir, "toc-exit-code.txt"), []byte("0\n"), 0644)
			_ = os.WriteFile(filepath.Join(childWorkDir, "toc-output.txt"), []byte("child delivered result\n"), 0644)
			_, _ = WriteSubAgentCompletionNotification(workspace, parentSessionID, SessionNotification{
				SessionID: "child-native-1",
				Agent:     agentName,
				Status:    session.StatusCompletedOK,
				ExitCode:  0,
				Prompt:    prompt,
				Output:    "child delivered result\n",
			})
		}()

		return &SubAgentSpawnResult{SessionID: "child-native-1"}, nil
	}

	var stdout bytes.Buffer
	err := RunNativeSession(NativeRunOptions{
		Mode:      "detached",
		Dir:       workDir,
		SessionID: "sess-native-parent",
		Agent:     "native-parent",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
		Prompt:    "Delegate the architecture review and continue when the result is ready.",
		SpawnFunc: spawnFunc,
	}, bytes.NewBuffer(nil), &stdout)
	if err != nil {
		t.Fatal(err)
	}

	if got := stdout.String(); got != "Spawned cto and waiting for completion.\nIntegrated child result.\n" {
		t.Fatalf("stdout = %q", got)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 model calls, got %d", callCount)
	}

	state, err := LoadStateInWorkspace(metaWorkspace, "sess-native-parent")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "completed" {
		t.Fatalf("state status = %q", state.Status)
	}
	last := state.Messages[len(state.Messages)-1]
	if last.Role != "assistant" || last.Content != "Integrated child result." {
		t.Fatalf("unexpected final message %#v", last)
	}
}

func TestRunNativeSession_MaxIterationsPromptsUser(t *testing.T) {
	// When the agent exhausts its iteration budget in an interactive session,
	// the runner should print a continuation prompt and wait for user input
	// instead of failing the session.
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()
	agentDir := filepath.Join(metaWorkspace, ".toc", "agents", "iter-agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a test agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set MaxIterations to 1 so the limit is hit immediately after one tool call cycle.
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-iter", &SessionConfig{
		Agent:   "iter-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools:  NativeToolNames(),
			MaxIterations: 1,
		},
	}); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch callCount {
		case 1:
			// First call: return a tool call so the loop uses its one iteration.
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-iter-1",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]interface{}{
								{
									"index": 0,
									"id":    "call-iter-1",
									"type":  "function",
									"function": map[string]interface{}{
										"name":      "Read",
										"arguments": `{"file_path":"` + filepath.Join(workDir, "notes.txt") + `"}`,
									},
								},
							},
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-iter-1",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "tool_calls",
					},
				},
			})
		case 2:
			// Second call (after user continues): return a text-only response.
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-iter-2",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "Continued successfully.",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-iter-2",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 11, "completion_tokens": 6, "total_tokens": 17},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
		default:
			t.Fatalf("unexpected request count %d", callCount)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	// Create a file for the Read tool to find.
	if err := os.WriteFile(filepath.Join(workDir, "notes.txt"), []byte("test content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Simulate user input: after the iteration limit message, the user
	// sends "keep going" followed by EOF.
	stdinContent := "keep going\n"
	stdin := strings.NewReader(stdinContent)

	var stdout bytes.Buffer
	err := RunNativeSession(NativeRunOptions{
		Dir:       workDir,
		SessionID: "sess-iter",
		Agent:     "iter-agent",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
		Prompt:    "Read notes.txt",
	}, stdin, &stdout)
	if err != nil {
		t.Fatalf("RunNativeSession should not fail: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Agent reached tool iteration limit (1)") {
		t.Fatalf("expected iteration limit message in stdout, got %q", out)
	}
	if !strings.Contains(out, "Continued successfully.") {
		t.Fatalf("expected continuation response in stdout, got %q", out)
	}

	state, err := LoadStateInWorkspace(metaWorkspace, "sess-iter")
	if err != nil {
		t.Fatal(err)
	}
	// Session should have completed successfully, not "failed".
	if state.Status == "failed" {
		t.Fatalf("session should not be in failed state, got status=%q error=%q", state.Status, state.LastError)
	}
}

func TestRunNativeSession_MaxIterationsDetachedFails(t *testing.T) {
	// In detached mode, hitting the iteration limit should fail the session
	// (no stdin to prompt).
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()
	agentDir := filepath.Join(metaWorkspace, ".toc", "agents", "iter-agent-d")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a test agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-iter-d", &SessionConfig{
		Agent:   "iter-agent-d",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools:  NativeToolNames(),
			MaxIterations: 1,
		},
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		// Always return a tool call so the loop never terminates naturally.
		writeSSEChunk(t, w, map[string]interface{}{
			"id":    "resp-d-1",
			"model": req.Model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"index": 0,
								"id":    "call-d-1",
								"type":  "function",
								"function": map[string]interface{}{
									"name":      "Read",
									"arguments": `{"file_path":"` + filepath.Join(workDir, "notes.txt") + `"}`,
								},
							},
						},
					},
				},
			},
		})
		writeSSEChunk(t, w, map[string]interface{}{
			"id":    "resp-d-1",
			"model": req.Model,
			"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": "tool_calls",
				},
			},
		})
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	if err := os.WriteFile(filepath.Join(workDir, "notes.txt"), []byte("test content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	err := RunNativeSession(NativeRunOptions{
		Mode:      "detached",
		Dir:       workDir,
		SessionID: "sess-iter-d",
		Agent:     "iter-agent-d",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
		Prompt:    "Read notes.txt",
	}, bytes.NewBuffer(nil), &stdout)

	if err == nil {
		t.Fatal("expected error for detached session hitting iteration limit")
	}
	if !strings.Contains(err.Error(), "exceeded max tool iterations") {
		t.Fatalf("unexpected error: %v", err)
	}

	state, err := LoadStateInWorkspace(metaWorkspace, "sess-iter-d")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "failed" {
		t.Fatalf("detached session should be failed, got %q", state.Status)
	}
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

func TestRunNativeSession_RejectsUnsupportedCustomModelWithoutOverride(t *testing.T) {
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a coding agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-native-unsupported", &SessionConfig{
		Agent:   "native-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "meta-llama/unknown",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	err := RunNativeSession(NativeRunOptions{
		Mode:      "detached",
		Dir:       workDir,
		SessionID: "sess-native-unsupported",
		Agent:     "native-agent",
		Workspace: metaWorkspace,
		Model:     "meta-llama/unknown",
		Prompt:    "hello",
	}, bytes.NewBuffer(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected unsupported native model to fail")
	}
	if got := err.Error(); !strings.Contains(got, "allow_custom_native_model") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunNativeSession_RecoversInterruptedTurnOnResume(t *testing.T) {
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a coding agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-native-recover", &SessionConfig{
		Agent:   "native-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveStateInWorkspace(metaWorkspace, "sess-native-recover", &State{
		Runtime:    runtimeinfo.NativeRuntime,
		SessionID:  "sess-native-recover",
		Agent:      "native-agent",
		Model:      "openai/gpt-4o-mini",
		Workspace:  metaWorkspace,
		SessionDir: workDir,
		Status:     "running",
		Messages: []Message{
			{Role: "system", Content: "You are a coding agent."},
			{Role: "user", Content: "Inspect the repo"},
		},
		PendingTurn: &TurnCheckpoint{
			Phase:  "awaiting_model",
			Prompt: "Inspect the repo",
		},
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSEChunk(t, w, map[string]interface{}{
			"id":    "resp-recover",
			"model": "openai/gpt-4o-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"role":    "assistant",
						"content": "Recovered and continued.",
					},
				},
			},
		})
		writeSSEChunk(t, w, map[string]interface{}{
			"id":    "resp-recover",
			"model": "openai/gpt-4o-mini",
			"usage": map[string]interface{}{"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6},
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": "stop",
				},
			},
		})
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	var stdout bytes.Buffer
	err := RunNativeSession(NativeRunOptions{
		Mode:      "detached",
		Dir:       workDir,
		SessionID: "sess-native-recover",
		Agent:     "native-agent",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
		Prompt:    "Continue from where you left off.",
		Resume:    true,
	}, bytes.NewBuffer(nil), &stdout)
	if err != nil {
		t.Fatal(err)
	}

	state, err := LoadStateInWorkspace(metaWorkspace, "sess-native-recover")
	if err != nil {
		t.Fatal(err)
	}
	if state.RecoveryCount != 1 {
		t.Fatalf("RecoveryCount = %d, want 1", state.RecoveryCount)
	}
	if state.PendingTurn != nil {
		t.Fatalf("expected pending turn to be cleared, got %#v", state.PendingTurn)
	}
	if state.LastRecovery == "" || !strings.Contains(state.LastRecovery, "waiting for the model response") {
		t.Fatalf("unexpected last recovery: %#v", state.LastRecovery)
	}

	sess := &session.Session{
		ID:          "sess-native-recover",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: MetadataDir(metaWorkspace, "sess-native-recover"),
	}
	parsed, err := LoadEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) < 2 {
		t.Fatalf("expected recovery and text events, got %#v", parsed.Events)
	}
	if parsed.Events[0].Step.Type != "recovery" {
		t.Fatalf("expected first event to be recovery, got %#v", parsed.Events[0].Step)
	}
}

func TestAccumulateUsage(t *testing.T) {
	tests := []struct {
		name       string
		state      *State
		resp       *chatResponse
		wantInput  int64
		wantOutput int64
		wantCacheR int64
		wantCacheW int64
	}{
		{
			name:  "nil state",
			state: nil,
			resp:  &chatResponse{},
		},
		{
			name:  "nil response",
			state: &State{},
			resp:  nil,
		},
		{
			name:  "basic tokens without cache details",
			state: &State{},
			resp: func() *chatResponse {
				r := &chatResponse{}
				r.Usage.PromptTokens = 10
				r.Usage.CompletionTokens = 5
				return r
			}(),
			wantInput:  10,
			wantOutput: 5,
		},
		{
			name:  "tokens with cache details",
			state: &State{},
			resp: func() *chatResponse {
				r := &chatResponse{}
				r.Usage.PromptTokens = 20
				r.Usage.CompletionTokens = 8
				r.Usage.PromptTokensDetails = &promptTokensDetails{
					CachedTokens:     12,
					CacheWriteTokens: 6,
				}
				return r
			}(),
			wantInput:  20,
			wantOutput: 8,
			wantCacheR: 12,
			wantCacheW: 6,
		},
		{
			name: "accumulates across multiple calls",
			state: &State{
				Usage: TokenUsageSnapshot{
					InputTokens:  10,
					OutputTokens: 5,
					CacheRead:    3,
					CacheCreate:  2,
				},
			},
			resp: func() *chatResponse {
				r := &chatResponse{}
				r.Usage.PromptTokens = 15
				r.Usage.CompletionTokens = 7
				r.Usage.PromptTokensDetails = &promptTokensDetails{
					CachedTokens:     9,
					CacheWriteTokens: 4,
				}
				return r
			}(),
			wantInput:  25,
			wantOutput: 12,
			wantCacheR: 12,
			wantCacheW: 6,
		},
		{
			name:  "nil prompt_tokens_details leaves cache at zero",
			state: &State{},
			resp: func() *chatResponse {
				r := &chatResponse{}
				r.Usage.PromptTokens = 10
				r.Usage.CompletionTokens = 5
				r.Usage.PromptTokensDetails = nil
				return r
			}(),
			wantInput:  10,
			wantOutput: 5,
			wantCacheR: 0,
			wantCacheW: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accumulateUsage(tt.state, tt.resp)
			if tt.state == nil {
				return
			}
			if tt.state.Usage.InputTokens != tt.wantInput {
				t.Errorf("InputTokens = %d, want %d", tt.state.Usage.InputTokens, tt.wantInput)
			}
			if tt.state.Usage.OutputTokens != tt.wantOutput {
				t.Errorf("OutputTokens = %d, want %d", tt.state.Usage.OutputTokens, tt.wantOutput)
			}
			if tt.state.Usage.CacheRead != tt.wantCacheR {
				t.Errorf("CacheRead = %d, want %d", tt.state.Usage.CacheRead, tt.wantCacheR)
			}
			if tt.state.Usage.CacheCreate != tt.wantCacheW {
				t.Errorf("CacheCreate = %d, want %d", tt.state.Usage.CacheCreate, tt.wantCacheW)
			}
		})
	}
}

func TestMergeStreamChunk_CacheTokens(t *testing.T) {
	tests := []struct {
		name          string
		resp          *chatResponse
		chunk         *chatStreamChunk
		wantCacheR    int64
		wantCacheW    int64
		wantNilDetail bool
	}{
		{
			name:  "nil chunk",
			resp:  &chatResponse{},
			chunk: nil,
		},
		{
			name:  "nil resp",
			resp:  nil,
			chunk: &chatStreamChunk{},
		},
		{
			name: "chunk with cache details populates resp",
			resp: &chatResponse{},
			chunk: func() *chatStreamChunk {
				c := &chatStreamChunk{}
				c.Usage.PromptTokens = 20
				c.Usage.CompletionTokens = 10
				c.Usage.TotalTokens = 30
				c.Usage.PromptTokensDetails = &promptTokensDetails{
					CachedTokens:     15,
					CacheWriteTokens: 5,
				}
				return c
			}(),
			wantCacheR: 15,
			wantCacheW: 5,
		},
		{
			name: "chunk without cache details preserves existing",
			resp: func() *chatResponse {
				r := &chatResponse{}
				r.Usage.PromptTokensDetails = &promptTokensDetails{
					CachedTokens:     10,
					CacheWriteTokens: 3,
				}
				return r
			}(),
			chunk: func() *chatStreamChunk {
				c := &chatStreamChunk{}
				c.Usage.PromptTokens = 25
				c.Usage.TotalTokens = 35
				c.Usage.PromptTokensDetails = nil
				return c
			}(),
			wantCacheR: 10,
			wantCacheW: 3,
		},
		{
			name: "chunk with zero usage does not overwrite",
			resp: func() *chatResponse {
				r := &chatResponse{}
				r.Usage.PromptTokens = 20
				r.Usage.PromptTokensDetails = &promptTokensDetails{
					CachedTokens:     8,
					CacheWriteTokens: 2,
				}
				return r
			}(),
			chunk: func() *chatStreamChunk {
				c := &chatStreamChunk{}
				// all usage fields zero — should not overwrite
				return c
			}(),
			wantCacheR: 8,
			wantCacheW: 2,
		},
		{
			name: "no cache details anywhere",
			resp: &chatResponse{},
			chunk: func() *chatStreamChunk {
				c := &chatStreamChunk{}
				c.Usage.PromptTokens = 10
				c.Usage.TotalTokens = 15
				return c
			}(),
			wantNilDetail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeStreamChunk(tt.resp, tt.chunk)
			if tt.resp == nil || tt.chunk == nil {
				return // just verifying no panic
			}
			d := tt.resp.Usage.PromptTokensDetails
			if tt.wantNilDetail {
				if d != nil {
					t.Fatalf("expected nil PromptTokensDetails, got %+v", d)
				}
				return
			}
			if d == nil {
				t.Fatal("expected non-nil PromptTokensDetails")
			}
			if d.CachedTokens != tt.wantCacheR {
				t.Errorf("CachedTokens = %d, want %d", d.CachedTokens, tt.wantCacheR)
			}
			if d.CacheWriteTokens != tt.wantCacheW {
				t.Errorf("CacheWriteTokens = %d, want %d", d.CacheWriteTokens, tt.wantCacheW)
			}
		})
	}
}

func TestRunNativeSession_InteractiveNotificationWhileWaiting(t *testing.T) {
	// When an interactive session is blocked waiting for user input, a
	// sub-agent completion notification should wake the loop, trigger a
	// model turn with the notification content, and then resume waiting
	// for user input.
	workDir := t.TempDir()
	metaWorkspace := t.TempDir()
	agentDir := filepath.Join(metaWorkspace, ".toc", "agents", "notify-agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, ".toc-native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".toc-native", "system-prompt.md"), []byte("You are a test agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionConfigInWorkspace(metaWorkspace, "sess-notify-interactive", &SessionConfig{
		Agent:   "notify-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch callCount {
		case 1:
			// Response to the initial prompt.
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-n-1",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "Waiting for sub-agent.",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-n-1",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 3, "total_tokens": 13},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
		case 2:
			// Response to the sub-agent completion notification.
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "user" || !strings.Contains(last.Content, "Sub-agent completion update.") {
				t.Fatalf("expected notification prompt, got role=%q content=%q", last.Role, last.Content)
			}
			if !strings.Contains(last.Content, "child output here") {
				t.Fatalf("expected child output in prompt, got %q", last.Content)
			}
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-n-2",
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": "Got the result.",
						},
					},
				},
			})
			writeSSEChunk(t, w, map[string]interface{}{
				"id":    "resp-n-2",
				"model": req.Model,
				"usage": map[string]interface{}{"prompt_tokens": 20, "completion_tokens": 4, "total_tokens": 24},
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			})
		default:
			t.Fatalf("unexpected request count %d", callCount)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_BASE_URL", server.URL)

	// Use a pipe for stdin so we control exactly when EOF arrives.
	stdinR, stdinW := io.Pipe()

	go func() {
		// Send the first user message immediately so the session has
		// work to do. After the model responds, the loop will block in
		// the select waiting for the next input.
		_, _ = stdinW.Write([]byte("Start working.\n"))

		// Give the session time to process and enter the select loop,
		// then write a notification file for the ticker to pick up.
		time.Sleep(500 * time.Millisecond)

		_, _ = WriteSubAgentCompletionNotification(metaWorkspace, "sess-notify-interactive", SessionNotification{
			SessionID: "child-interactive-1",
			Agent:     "cto",
			Status:    session.StatusCompletedOK,
			ExitCode:  0,
			Prompt:    "Do the review.",
			Output:    "child output here",
		})

		// After the notification is processed, close stdin to end the
		// session. Give enough time for the ticker to fire and the model
		// response to complete.
		time.Sleep(4 * time.Second)
		stdinW.Close()
	}()

	var stdout bytes.Buffer
	err := RunNativeSession(NativeRunOptions{
		Dir:       workDir,
		SessionID: "sess-notify-interactive",
		Agent:     "notify-agent",
		Workspace: metaWorkspace,
		Model:     "openai/gpt-4o-mini",
	}, stdinR, &stdout)
	if err != nil {
		t.Fatalf("RunNativeSession failed: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected 2 model calls (initial + notification), got %d", callCount)
	}

	out := stdout.String()
	if !strings.Contains(out, "Waiting for sub-agent.") {
		t.Fatalf("expected initial response in stdout, got %q", out)
	}
	if !strings.Contains(out, "Got the result.") {
		t.Fatalf("expected notification response in stdout, got %q", out)
	}

	state, err := LoadStateInWorkspace(metaWorkspace, "sess-notify-interactive")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "completed" {
		t.Fatalf("expected completed status, got %q", state.Status)
	}
	// Verify the notification prompt was injected into the message history.
	foundNotification := false
	for _, msg := range state.Messages {
		if msg.Role == "user" && strings.Contains(msg.Content, "Sub-agent completion update.") {
			foundNotification = true
			break
		}
	}
	if !foundNotification {
		t.Fatal("expected notification prompt in message history")
	}
}
