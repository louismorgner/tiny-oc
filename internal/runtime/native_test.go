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

func TestBuildSkillCatalog(t *testing.T) {
	dir := t.TempDir()

	// Empty dir → empty catalog.
	if got := buildSkillCatalog(dir); got != "" {
		t.Fatalf("expected empty catalog for empty dir, got %q", got)
	}

	// Non-existent dir → empty catalog.
	if got := buildSkillCatalog(filepath.Join(dir, "nonexistent")); got != "" {
		t.Fatalf("expected empty catalog for missing dir, got %q", got)
	}

	// One valid skill.
	skillDir := filepath.Join(dir, "commit")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: commit\ndescription: Generate a conventional commit message and create a commit.\n---\n\nInstructions here.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}

	catalog := buildSkillCatalog(dir)
	if catalog == "" {
		t.Fatal("expected non-empty catalog")
	}
	if !strings.Contains(catalog, `name="commit"`) {
		t.Errorf("catalog missing skill name: %q", catalog)
	}
	if !strings.Contains(catalog, "conventional commit") {
		t.Errorf("catalog missing description: %q", catalog)
	}
	if !strings.Contains(catalog, "<available_skills>") {
		t.Errorf("catalog missing XML wrapper: %q", catalog)
	}

	// Skill with XML-special characters in description is escaped.
	specialDir := filepath.Join(dir, "special")
	if err := os.MkdirAll(specialDir, 0755); err != nil {
		t.Fatal(err)
	}
	specialMD := "---\nname: special\ndescription: Use when code has <errors> & warnings.\n---\n"
	if err := os.WriteFile(filepath.Join(specialDir, "SKILL.md"), []byte(specialMD), 0644); err != nil {
		t.Fatal(err)
	}
	catalog2 := buildSkillCatalog(dir)
	if !strings.Contains(catalog2, "&lt;errors&gt;") {
		t.Errorf("catalog did not escape XML in description: %q", catalog2)
	}
	if !strings.Contains(catalog2, "&amp;") {
		t.Errorf("catalog did not escape & in description: %q", catalog2)
	}

	// Skill without description is skipped.
	nodescDir := filepath.Join(dir, "nodesc")
	if err := os.MkdirAll(nodescDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodescDir, "SKILL.md"), []byte("---\nname: nodesc\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	catalog3 := buildSkillCatalog(dir)
	if strings.Contains(catalog3, `name="nodesc"`) {
		t.Errorf("catalog should skip skill with no description: %q", catalog3)
	}
}

func TestEnsureSystemPromptInjectsSkillCatalog(t *testing.T) {
	sessionDir := t.TempDir()
	nativeDir := filepath.Join(sessionDir, ".toc-native")
	skillsDir := filepath.Join(nativeDir, "skills")

	if err := os.MkdirAll(nativeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nativeDir, "system-prompt.md"), []byte("You are an agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// No skills yet — system prompt alone.
	state := &State{SessionDir: sessionDir}
	if err := ensureSystemPrompt(state); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 1 || state.Messages[0].Role != "system" {
		t.Fatalf("expected system message, got %+v", state.Messages)
	}
	if strings.Contains(state.Messages[0].Content, "available_skills") {
		t.Errorf("catalog should not appear when no skills present")
	}

	// Add a skill — catalog should appear on next fresh state.
	if err := os.MkdirAll(filepath.Join(skillsDir, "commit"), 0755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: commit\ndescription: Create a git commit.\n---\nInstructions.\n"
	if err := os.WriteFile(filepath.Join(skillsDir, "commit", "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}

	state2 := &State{SessionDir: sessionDir}
	if err := ensureSystemPrompt(state2); err != nil {
		t.Fatal(err)
	}
	content := state2.Messages[0].Content
	if !strings.Contains(content, "You are an agent.") {
		t.Errorf("system prompt missing base content: %q", content)
	}
	if !strings.Contains(content, "<available_skills>") {
		t.Errorf("system prompt missing skill catalog: %q", content)
	}
	if !strings.Contains(content, `name="commit"`) {
		t.Errorf("system prompt missing skill name: %q", content)
	}

	// No system-prompt.md but skills present — catalog becomes the system prompt.
	sessionDir3 := t.TempDir()
	skillsDir3 := filepath.Join(sessionDir3, ".toc-native", "skills", "review")
	if err := os.MkdirAll(skillsDir3, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir3, "SKILL.md"), []byte("---\nname: review\ndescription: Review code.\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	state3 := &State{SessionDir: sessionDir3}
	if err := ensureSystemPrompt(state3); err != nil {
		t.Fatal(err)
	}
	if len(state3.Messages) != 1 || !strings.Contains(state3.Messages[0].Content, "available_skills") {
		t.Errorf("expected skill-only system prompt, got %+v", state3.Messages)
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
