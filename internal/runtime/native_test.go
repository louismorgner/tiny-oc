package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestGetNativeProvider(t *testing.T) {
	provider, err := Get(runtimeinfo.NativeRuntime)
	if err != nil {
		t.Fatal(err)
	}
	if provider.Name() != runtimeinfo.NativeRuntime {
		t.Fatalf("provider.Name() = %q", provider.Name())
	}
}

func TestNativePrepareSessionWritesPrompt(t *testing.T) {
	provider, err := Get(runtimeinfo.NativeRuntime)
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
		Agent:   "native-tester",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "test-model",
		Compose: []string{"extra.md"},
		RuntimeConfig: SessionRuntimeOptions{
			EnabledTools: NativeToolNames(),
		},
	}

	if err := provider.PrepareSession(workDir, agentDir, cfg, "sess-native"); err != nil {
		t.Fatal(err)
	}

	promptPath := filepath.Join(workDir, ".toc-native", "system-prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "agent native-tester sess-native") {
		t.Fatalf("system prompt missing substituted content: %q", content)
	}
	if !strings.Contains(content, "model test-model") {
		t.Fatalf("system prompt missing composed model content: %q", content)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".toc-native", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no duplicated native session config, got err=%v", err)
	}
}

func TestNativeProvider_UsesEventLogAsSessionLog(t *testing.T) {
	metaDir := t.TempDir()
	sess := &session.Session{
		ID:          "sess-native",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: metaDir,
	}

	provider, err := Get(runtimeinfo.NativeRuntime)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(metaDir, "events.jsonl")
	if got := provider.SessionLogPath(sess); got != want {
		t.Fatalf("SessionLogPath() = %q, want %q", got, want)
	}
	if got := provider.ExpectedSessionLogPath(sess); got != want {
		t.Fatalf("ExpectedSessionLogPath() = %q, want %q", got, want)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	sess := &session.Session{ID: "sess-state", MetadataDir: t.TempDir()}
	state := &State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: "sess-state",
		Agent:     "native-agent",
		Model:     "test-model",
		Status:    "running",
		Messages: []Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "hello"},
		},
	}

	if err := SaveState(sess, state); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadState(sess)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != StateVersion {
		t.Fatalf("Version = %d, want %d", loaded.Version, StateVersion)
	}
	if loaded.Agent != "native-agent" || len(loaded.Messages) != 2 {
		t.Fatalf("loaded state = %#v", loaded)
	}
	if loaded.CreatedAt.IsZero() || loaded.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps, got %#v", loaded)
	}
}

func TestNativeParseSessionLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	ts := time.Date(2026, 3, 25, 18, 0, 0, 0, time.UTC)

	sess := &session.Session{ID: "sess-native", MetadataDir: dir}
	if err := AppendEvent(sess, Event{Timestamp: ts, Step: Step{Type: "text", Content: "boot"}}); err != nil {
		t.Fatal(err)
	}

	provider, err := Get(runtimeinfo.NativeRuntime)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := provider.ParseSessionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 1 || parsed.Steps[0].Content != "boot" {
		t.Fatalf("parsed log = %#v", parsed)
	}
	if !parsed.FirstTS.Equal(ts) || !parsed.LastTS.Equal(ts) {
		t.Fatalf("parsed timestamps = %v -> %v", parsed.FirstTS, parsed.LastTS)
	}
}
