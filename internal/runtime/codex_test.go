package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestCodexPrepareSessionWritesAgentsAndGitRepo(t *testing.T) {
	provider, err := Get(runtimeinfo.CodexRuntime)
	if err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "agent.md"), []byte("agent {{.AgentName}} {{.SessionID}}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "extra.md"), []byte("model {{.Model}}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Agent:   "codex-tester",
		Runtime: runtimeinfo.CodexRuntime,
		Model:   "gpt-5-codex",
		Compose: []string{"extra.md"},
	}

	if err := provider.PrepareSession(workDir, t.TempDir(), cfg, "sess-codex"); err != nil {
		t.Fatal(err)
	}

	agentsMD, err := os.ReadFile(filepath.Join(workDir, codexInstructionFile))
	if err != nil {
		t.Fatal(err)
	}
	content := string(agentsMD)
	if !strings.Contains(content, "agent codex-tester sess-codex") {
		t.Fatalf("AGENTS.md missing substituted content: %q", content)
	}
	if !strings.Contains(content, "model gpt-5-codex") {
		t.Fatalf("AGENTS.md missing composed model content: %q", content)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err != nil {
		t.Fatalf("expected git repo for codex runtime: %v", err)
	}
}

func TestCodexProvider_UsesLocalExecEventLog(t *testing.T) {
	workDir := t.TempDir()
	logPath := filepath.Join(workDir, codexEventsFile)
	content := `{"type":"thread.started","thread_id":"codex-thread-1"}
{"type":"item.completed","item":{"id":"msg-1","type":"agent_message","text":"done"}}
`
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	sess := &session.Session{
		ID:              "sess-codex",
		Runtime:         runtimeinfo.CodexRuntime,
		MetadataDir:     t.TempDir(),
		WorkspacePath:   workDir,
		ParentSessionID: "parent-1",
	}

	provider, err := Get(runtimeinfo.CodexRuntime)
	if err != nil {
		t.Fatal(err)
	}

	if got := provider.SessionLogPath(sess); got != logPath {
		t.Fatalf("SessionLogPath() = %q, want %q", got, logPath)
	}
	if got := provider.ExpectedSessionLogPath(sess); got != logPath {
		t.Fatalf("ExpectedSessionLogPath() = %q, want %q", got, logPath)
	}
}

func TestParseCodexExecLog(t *testing.T) {
	provider, err := Get(runtimeinfo.CodexRuntime)
	if err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), codexEventsFile)
	content := `{"type":"thread.started","thread_id":"codex-thread-1"}
{"type":"item.completed","item":{"id":"reason-1","type":"reasoning","text":"Inspecting runtime support"}}
{"type":"item.completed","item":{"id":"cmd-1","type":"command_execution","command":"rg --files","aggregated_output":"main.go","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"patch-1","type":"file_change","changes":[{"path":"README.md","kind":"update"}],"status":"completed"}}
{"type":"item.completed","item":{"id":"msg-1","type":"agent_message","text":"done"}}
`
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	parsed, err := provider.ParseSessionLog(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Steps) != 4 {
		t.Fatalf("expected 4 parsed steps, got %#v", parsed.Steps)
	}
	if parsed.Steps[0].Type != "thinking" {
		t.Fatalf("expected reasoning step, got %#v", parsed.Steps[0])
	}
	if parsed.Steps[1].Tool != "Bash" || parsed.Steps[1].Command != "rg --files" {
		t.Fatalf("expected bash step, got %#v", parsed.Steps[1])
	}
	if parsed.Steps[2].Tool != "Edit" || parsed.Steps[2].Path != "README.md" {
		t.Fatalf("expected file change step, got %#v", parsed.Steps[2])
	}
	if parsed.Steps[3].Content != "done" {
		t.Fatalf("expected final text step, got %#v", parsed.Steps[3])
	}
}

func TestParseCodexRolloutLogAndDiscoverByWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	workDir := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "03", "27")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(sessionsDir, "rollout-2026-03-27T11-00-00-codex-thread-42.jsonl")
	firstLine, err := json.Marshal(map[string]interface{}{
		"timestamp": "2026-03-27T11:00:00Z",
		"type":      "session_meta",
		"payload": map[string]interface{}{
			"id":        "codex-thread-42",
			"timestamp": "2026-03-27T11:00:00Z",
			"cwd":       workDir,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondLine, err := json.Marshal(map[string]interface{}{
		"timestamp": "2026-03-27T11:00:01Z",
		"type":      "event_msg",
		"payload": map[string]interface{}{
			"type": "agent_reasoning",
			"text": "Checking runtime support",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	thirdLine, err := json.Marshal(map[string]interface{}{
		"timestamp": "2026-03-27T11:00:02Z",
		"type":      "response_item",
		"payload": map[string]interface{}{
			"type":      "function_call",
			"name":      "shell_command",
			"arguments": `{"command":"git status"}`,
			"call_id":   "call-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fourthLine, err := json.Marshal(map[string]interface{}{
		"timestamp": "2026-03-27T11:00:03Z",
		"type":      "response_item",
		"payload": map[string]interface{}{
			"type":    "function_call_output",
			"call_id": "call-1",
			"output":  "Exit code: 0\nWall time: 0.1 seconds\nOutput:\nclean\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, append(append(append(append(firstLine, '\n'), secondLine...), '\n'), append(append(thirdLine, '\n'), append(fourthLine, '\n')...)...), 0644); err != nil {
		t.Fatal(err)
	}

	sess := &session.Session{
		ID:            "sess-codex",
		Runtime:       runtimeinfo.CodexRuntime,
		MetadataDir:   t.TempDir(),
		WorkspacePath: workDir,
		CreatedAt:     time.Date(2026, 3, 27, 10, 59, 0, 0, time.UTC),
	}

	provider, err := Get(runtimeinfo.CodexRuntime)
	if err != nil {
		t.Fatal(err)
	}

	discovered := provider.SessionLogPath(sess)
	if discovered != logPath {
		t.Fatalf("SessionLogPath() = %q, want %q", discovered, logPath)
	}

	parsed, err := provider.ParseSessionLog(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Steps) != 2 {
		t.Fatalf("expected 2 parsed rollout steps, got %#v", parsed.Steps)
	}
	if parsed.Steps[0].Type != "thinking" {
		t.Fatalf("expected thinking step, got %#v", parsed.Steps[0])
	}
	if parsed.Steps[1].Tool != "Bash" || parsed.Steps[1].Command != "git status" {
		t.Fatalf("expected bash step from rollout log, got %#v", parsed.Steps[1])
	}
}
