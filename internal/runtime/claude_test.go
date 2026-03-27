package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/session"
)

func TestClaudePrepareSession_ProvisionsInstructionsAndHooks(t *testing.T) {
	provider, err := Get(DefaultRuntime)
	if err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	agentDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "agent.md"), []byte("agent {{.AgentName}} {{.SessionID}}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "extra.md"), []byte("model {{.Model}}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Agent:   "tester",
		Runtime: DefaultRuntime,
		Model:   "sonnet",
		Compose: []string{"extra.md"},
		Context: []string{"notes.md"},
		OnEnd:   "persist state",
		Permissions: agent.Permissions{
			Filesystem: agent.FilesystemPermissions{Read: agent.PermAsk, Write: agent.PermOn, Execute: agent.PermOn},
		},
	}

	if err := provider.PrepareSession(workDir, agentDir, cfg, "sess-123"); err != nil {
		t.Fatal(err)
	}

	claudeMD, err := os.ReadFile(filepath.Join(workDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(claudeMD)
	if !strings.Contains(content, "agent tester sess-123") {
		t.Fatalf("CLAUDE.md missing substituted agent/session content: %q", content)
	}
	if !strings.Contains(content, "model sonnet") {
		t.Fatalf("CLAUDE.md missing composed model content: %q", content)
	}
	if _, err := os.Stat(filepath.Join(workDir, "agent.md")); !os.IsNotExist(err) {
		t.Fatal("agent.md should be removed after provisioning")
	}

	settings, err := os.ReadFile(filepath.Join(workDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	settingsText := string(settings)
	if !strings.Contains(settingsText, "PostToolUse") {
		t.Fatal("expected PostToolUse hook in settings")
	}
	if !strings.Contains(settingsText, "PreToolUse") {
		t.Fatal("expected PreToolUse hook in settings")
	}
	if !strings.Contains(settingsText, "SessionEnd") {
		t.Fatal("expected SessionEnd hook in settings")
	}
}

func TestClaudeProvider_SkillsDirAndPostSessionSync(t *testing.T) {
	provider, err := Get(DefaultRuntime)
	if err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	agentDir := t.TempDir()
	if got := provider.SkillsDir(workDir); got != filepath.Join(workDir, ".claude", "skills") {
		t.Fatalf("SkillsDir() = %q", got)
	}

	if err := os.WriteFile(filepath.Join(workDir, "CLAUDE.md"), []byte("updated"), 0644); err != nil {
		t.Fatal(err)
	}
	synced, err := provider.PostSessionSync(workDir, agentDir, []string{"agent.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != "agent.md" {
		t.Fatalf("PostSessionSync() = %#v", synced)
	}

	data, err := os.ReadFile(filepath.Join(agentDir, "agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "updated" {
		t.Fatalf("synced agent.md = %q, want %q", string(data), "updated")
	}
}

func TestBuildClaudeArgs_OmitsDefaultModelAlias(t *testing.T) {
	args := buildClaudeArgs(LaunchOptions{Model: "default"})
	if strings.Contains(strings.Join(args, " "), "--model") {
		t.Fatalf("buildClaudeArgs() should omit --model for default alias, got %#v", args)
	}
}

func TestBuildClaudeArgs_IncludesExplicitModel(t *testing.T) {
	args := buildClaudeArgs(LaunchOptions{Model: "opus"})
	if !strings.Contains(strings.Join(args, " "), "--model opus") {
		t.Fatalf("buildClaudeArgs() should include explicit model, got %#v", args)
	}
}

func TestSaveAndLoadEventLog(t *testing.T) {
	ts1 := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(5 * time.Second)
	sess := &session.Session{ID: "sess-1", MetadataDir: t.TempDir()}

	parsed := &ParsedLog{
		Events: []Event{
			{Timestamp: ts1, Step: Step{Type: "tool", Tool: "Read", Path: "main.go"}},
			{Timestamp: ts2, Step: Step{Type: "text", Content: "done"}},
		},
		Steps:   []Step{{Type: "tool", Tool: "Read", Path: "main.go"}, {Type: "text", Content: "done"}},
		FirstTS: ts1,
		LastTS:  ts2,
	}

	if err := SaveEventLog(sess, parsed); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 || len(loaded.Steps) != 2 {
		t.Fatalf("loaded events/steps = %d/%d", len(loaded.Events), len(loaded.Steps))
	}
	if !loaded.FirstTS.Equal(ts1) || !loaded.LastTS.Equal(ts2) {
		t.Fatalf("loaded timestamps = %v -> %v", loaded.FirstTS, loaded.LastTS)
	}
}

func TestEnsureEventLog_BuildsCacheFromClaudeLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	workDir := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionID := "sess-123"
	encoded := strings.NewReplacer("/", "-", "_", "-").Replace(workDir)
	projectDir := filepath.Join(home, ".claude", "projects", encoded)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	line, err := json.Marshal(map[string]interface{}{
		"type":      "assistant",
		"timestamp": "2026-03-25T12:00:00Z",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "tool_use", "name": "Read", "input": map[string]string{"file_path": "main.go"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), append(line, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	metaDir := filepath.Join(t.TempDir(), "meta")
	sess := &session.Session{
		ID:            sessionID,
		Runtime:       DefaultRuntime,
		MetadataDir:   metaDir,
		WorkspacePath: workDir,
	}

	parsed, err := EnsureEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 1 || parsed.Events[0].Step.Tool != "Read" {
		t.Fatalf("parsed events = %#v", parsed.Events)
	}

	cachePath := EventLogPath(sess)
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cached event log at %s: %v", cachePath, err)
	}

	loaded, err := LoadEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Step.Path != "main.go" {
		t.Fatalf("loaded cached events = %#v", loaded.Events)
	}
}

func TestEnsureEventLog_UsesPartialCacheWhileActive(t *testing.T) {
	sess := &session.Session{
		ID:            "sess-active",
		Runtime:       DefaultRuntime,
		MetadataDir:   t.TempDir(),
		Status:        session.StatusActive,
		WorkspacePath: t.TempDir(),
	}
	if err := AppendEvent(sess, Event{Step: Step{Type: "text", Content: "partial"}}); err != nil {
		t.Fatal(err)
	}

	parsed, err := EnsureEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 1 || parsed.Events[0].Step.Content != "partial" {
		t.Fatalf("active EnsureEventLog() = %#v", parsed.Events)
	}
	if got := EventCount(sess); got != 1 {
		t.Fatalf("EventCount() = %d, want 1", got)
	}
}
