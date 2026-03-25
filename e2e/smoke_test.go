package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var tocBin string
var mockClaudeBin string

func TestMain(m *testing.M) {
	// Build toc binary.
	tmp, err := os.MkdirTemp("", "toc-e2e-bins-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	repoRoot := findRepoRoot()

	tocBin = filepath.Join(tmp, "toc")
	cmd := exec.Command("go", "build", "-o", tocBin, ".")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build toc: %v\n%s\n", err, out)
		os.Exit(1)
	}

	mockClaudeBin = filepath.Join(tmp, "claude")
	cmd = exec.Command("go", "build", "-o", mockClaudeBin, "./e2e/testutil/mockclaude")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build mock claude: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func findRepoRoot() string {
	// Walk up from test file location to find go.mod.
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Fallback: try relative to this test package.
			abs, _ := filepath.Abs("..")
			return abs
		}
		dir = parent
	}
}

// tocCmd creates an exec.Cmd for running toc in the given workspace dir.
// The mock claude binary directory is prepended to PATH.
func tocCmd(workDir string, args ...string) *exec.Cmd {
	cmd := exec.Command(tocBin, args...)
	cmd.Dir = workDir
	cmd.Env = pathEnv(workDir)
	return cmd
}

// pathEnv returns an environment with mock claude on PATH and clean HOME.
func pathEnv(workDir string) []string {
	mockDir := filepath.Dir(mockClaudeBin)
	env := []string{
		"PATH=" + mockDir + ":" + os.Getenv("PATH"),
		"HOME=" + workDir,
		"MOCK_CLAUDE_OUTPUT=mock claude output",
		"MOCK_CLAUDE_EXIT_CODE=0",
	}
	// Preserve essentials for Go to work.
	for _, key := range []string{"GOPATH", "GOROOT", "TMPDIR", "USER"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	return env
}

// newWorkspace creates a temp directory for a test.
func newWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// runToc runs toc and returns combined output. Fails the test on error.
func runToc(t *testing.T, workDir string, args ...string) string {
	t.Helper()
	cmd := tocCmd(workDir, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("toc %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// runTocErr runs toc and returns output + error. Does not fail on error.
func runTocErr(t *testing.T, workDir string, args ...string) (string, error) {
	t.Helper()
	cmd := tocCmd(workDir, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runTocWithEnv runs toc with additional env vars.
func runTocWithEnv(t *testing.T, workDir string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := tocCmd(workDir, args...)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("toc %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// runTocWithEnvErr runs toc with additional env vars and returns error.
func runTocWithEnvErr(t *testing.T, workDir string, extraEnv []string, args ...string) (string, error) {
	t.Helper()
	cmd := tocCmd(workDir, args...)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// initWorkspace runs toc init in a fresh workspace.
func initWorkspace(t *testing.T, dir, name string) {
	t.Helper()
	runToc(t, dir, "init", "--name", name)
}

// createAgent writes an agent config directly (bypasses interactive create).
func createAgent(t *testing.T, dir, name, model string, perms *agentPerms) {
	t.Helper()
	agentDir := filepath.Join(dir, ".toc", "agents", name)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := map[string]interface{}{
		"runtime": "claude-code",
		"name":    name,
		"model":   model,
	}
	if perms != nil {
		p := map[string]interface{}{}
		if len(perms.SubAgents) > 0 {
			p["sub-agents"] = perms.SubAgents
		}
		if len(perms.Integrations) > 0 {
			p["integrations"] = perms.Integrations
		}
		cfg["permissions"] = p
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Convert JSON to YAML-compatible format (Go's yaml.Unmarshal handles JSON).
	if err := os.WriteFile(filepath.Join(agentDir, "oc-agent.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("# "+name+"\nTest agent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

type agentPerms struct {
	SubAgents    map[string]string   `json:"sub-agents,omitempty"`
	Integrations map[string][]string `json:"integrations,omitempty"`
}

// --- Test Cases ---

func TestSmoke_InitCreatesWorkspace(t *testing.T) {
	dir := newWorkspace(t)
	out := runToc(t, dir, "init", "--name", "test-project")

	if !strings.Contains(out, "Initialized workspace") {
		t.Errorf("expected init success message, got: %s", out)
	}

	// Verify .toc directory structure.
	for _, path := range []string{
		".toc/config.yaml",
		".toc/agents",
		".toc/skills",
	} {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist after init", path)
		}
	}

	// Verify config content.
	cfgData, err := os.ReadFile(filepath.Join(dir, ".toc/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfgData), "test-project") {
		t.Errorf("config.yaml should contain workspace name, got: %s", cfgData)
	}

	// Double init should fail.
	_, err = runTocErr(t, dir, "init", "--name", "again")
	if err == nil {
		t.Error("expected double init to fail")
	}
}

func TestSmoke_AgentCreateAndList(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")

	// Create an agent by writing files directly (agent create is interactive).
	createAgent(t, dir, "reviewer", "sonnet", nil)

	// List should show the agent.
	out := runToc(t, dir, "agent", "list")
	if !strings.Contains(out, "reviewer") {
		t.Errorf("expected agent list to show 'reviewer', got: %s", out)
	}
	if !strings.Contains(out, "sonnet") {
		t.Errorf("expected agent list to show model 'sonnet', got: %s", out)
	}

	// Create a second agent.
	createAgent(t, dir, "coder", "opus", nil)
	out = runToc(t, dir, "agent", "list")
	if !strings.Contains(out, "coder") || !strings.Contains(out, "reviewer") {
		t.Errorf("expected both agents in list, got: %s", out)
	}
}

func TestSmoke_AgentSpawnAndComplete(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")
	createAgent(t, dir, "worker", "sonnet", nil)

	// Spawn launches mock claude, which exits immediately.
	out := runToc(t, dir, "agent", "spawn", "worker")

	if !strings.Contains(out, "worker") {
		t.Errorf("expected agent name in spawn output, got: %s", out)
	}
	if !strings.Contains(out, "Session:") {
		t.Errorf("expected session ID in spawn output, got: %s", out)
	}
	if !strings.Contains(out, "Resume with:") {
		t.Errorf("expected resume hint in spawn output, got: %s", out)
	}

	// Verify session was recorded.
	sessData, err := os.ReadFile(filepath.Join(dir, ".toc/sessions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sessData), "worker") {
		t.Errorf("expected session record for worker, got: %s", sessData)
	}
}

func TestSmoke_SubAgentSpawnStatusOutput(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")

	// Create parent agent that can spawn the worker.
	createAgent(t, dir, "parent", "sonnet", &agentPerms{
		SubAgents: map[string]string{"worker": "on"},
	})
	createAgent(t, dir, "worker", "sonnet", nil)

	// Simulate runtime context (as if running inside a parent session).
	parentSessionID := "parent-session-00000000-0000-0000-0000-000000000001"
	runtimeEnv := []string{
		"TOC_WORKSPACE=" + dir,
		"TOC_AGENT=parent",
		"TOC_SESSION_ID=" + parentSessionID,
	}

	// We need a sessions.yaml entry for the parent session.
	sessDir := filepath.Join(dir, ".toc")
	os.MkdirAll(sessDir, 0755)
	os.WriteFile(filepath.Join(sessDir, "sessions.yaml"), []byte(fmt.Sprintf(
		"sessions:\n- id: %s\n  agent: parent\n  created_at: 2026-01-01T00:00:00Z\n  workspace_path: %s\n  status: active\n",
		parentSessionID, dir,
	)), 0600)

	// Spawn sub-agent.
	out := runTocWithEnv(t, dir, runtimeEnv, "runtime", "spawn", "worker", "--prompt", "do the thing")
	if !strings.Contains(out, "Sub-agent") || !strings.Contains(out, "spawned") {
		t.Errorf("expected spawn success message, got: %s", out)
	}

	// Extract session ID from output.
	subSessionID := extractSessionID(out)
	if subSessionID == "" {
		t.Fatal("could not extract sub-agent session ID from output")
	}

	// Wait briefly for the detached process to finish.
	waitForSubAgent(t, dir, subSessionID, 10*time.Second)

	// Check status.
	statusOut := runTocWithEnv(t, dir, runtimeEnv, "runtime", "status", subSessionID, "--json")
	var status map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(statusOut)), &status); err != nil {
		t.Fatalf("failed to parse status JSON: %v\nraw: %s", err, statusOut)
	}

	statusStr, _ := status["status"].(string)
	if statusStr != "completed-success" && statusStr != "completed" {
		t.Errorf("expected completed status, got: %s", statusStr)
	}

	// Read output.
	outputOut := runTocWithEnv(t, dir, runtimeEnv, "runtime", "output", subSessionID)
	if !strings.Contains(outputOut, "mock claude output") {
		t.Errorf("expected mock output, got: %s", outputOut)
	}
}

func TestSmoke_SubAgentCanSpawnDenied(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")

	// Parent agent with NO sub-agent permissions.
	createAgent(t, dir, "locked", "sonnet", nil)
	createAgent(t, dir, "target", "sonnet", nil)

	runtimeEnv := []string{
		"TOC_WORKSPACE=" + dir,
		"TOC_AGENT=locked",
		"TOC_SESSION_ID=locked-session-001",
	}

	out, err := runTocWithEnvErr(t, dir, runtimeEnv, "runtime", "spawn", "target", "--prompt", "test")
	if err == nil {
		t.Errorf("expected permission denied error, but command succeeded: %s", out)
	}
	if !strings.Contains(out, "not allowed to spawn") {
		t.Errorf("expected 'not allowed to spawn' error, got: %s", out)
	}
}

func TestSmoke_ContextSyncOnSessionEnd(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")

	// Create agent with context sync pattern.
	agentDir := filepath.Join(dir, ".toc", "agents", "syncer")
	os.MkdirAll(agentDir, 0755)

	cfg := `runtime: claude-code
name: syncer
model: sonnet
context:
  - notes.md
`
	os.WriteFile(filepath.Join(agentDir, "oc-agent.yaml"), []byte(cfg), 0644)
	os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("# syncer\n"), 0644)

	// Tell mock claude to write a notes.md file in the session workspace.
	cmd := tocCmd(dir, "agent", "spawn", "syncer")
	cmd.Env = append(pathEnv(dir),
		"MOCK_CLAUDE_WRITE_FILE=notes.md",
		"MOCK_CLAUDE_WRITE_CONTENT=session notes here",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("spawn failed: %v\n%s", err, out)
	}

	// Check that notes.md was synced back to the agent template.
	syncedPath := filepath.Join(agentDir, "notes.md")
	data, err := os.ReadFile(syncedPath)
	if err != nil {
		t.Fatalf("expected notes.md to be synced back to agent dir, got error: %v\nspawn output: %s", err, out)
	}
	if !strings.Contains(string(data), "session notes here") {
		t.Errorf("synced file has wrong content: %s", data)
	}
}

func TestSmoke_HookScriptGeneration(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")

	// Create agent with context sync + permissions (triggers hook generation).
	agentDir := filepath.Join(dir, ".toc", "agents", "hooked")
	os.MkdirAll(agentDir, 0755)

	cfg := `runtime: claude-code
name: hooked
model: sonnet
context:
  - docs/
permissions:
  filesystem:
    read: "on"
    write: ask
    execute: "off"
`
	os.WriteFile(filepath.Join(agentDir, "oc-agent.yaml"), []byte(cfg), 0644)
	os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("# hooked\n"), 0644)

	out := runToc(t, dir, "agent", "spawn", "hooked")

	// Find the session workspace from the output to check generated files.
	wsPath := extractWorkspacePath(out)
	if wsPath == "" {
		t.Fatal("could not extract workspace path from spawn output")
	}

	// Verify hook scripts were generated.
	for _, file := range []string{
		".claude/settings.json",
		".claude/toc-sync.sh",
		".claude/toc-permissions.sh",
	} {
		full := filepath.Join(wsPath, file)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to be generated in session workspace", file)
		}
	}

	// Verify settings.json is valid JSON.
	settingsData, err := os.ReadFile(filepath.Join(wsPath, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Errorf("settings.json is not valid JSON: %v", err)
	}

	// Verify permission script is executable.
	info, err := os.Stat(filepath.Join(wsPath, ".claude/toc-permissions.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("toc-permissions.sh should be executable")
	}
}

func TestSmoke_AuditLogWritten(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")
	createAgent(t, dir, "audited", "sonnet", nil)

	runToc(t, dir, "agent", "spawn", "audited")

	// Check audit log.
	auditData, err := os.ReadFile(filepath.Join(dir, ".toc/audit.log"))
	if err != nil {
		t.Fatalf("expected audit.log to exist: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(auditData)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 audit entries (init + spawn), got %d", len(lines))
	}

	// Verify each line is valid JSON.
	var foundInit, foundSpawn bool
	for _, line := range lines {
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("invalid audit log line: %v\nline: %s", err, line)
			continue
		}

		action, _ := event["action"].(string)
		if action == "workspace.init" {
			foundInit = true
		}
		if action == "session.spawn" {
			foundSpawn = true
			details, _ := event["details"].(map[string]interface{})
			if details == nil {
				t.Error("session.spawn event has no details")
			} else if details["agent"] != "audited" {
				t.Errorf("expected agent=audited in audit, got: %v", details["agent"])
			}
		}
	}

	if !foundInit {
		t.Error("no workspace.init entry in audit log")
	}
	if !foundSpawn {
		t.Error("no session.spawn entry in audit log")
	}
}

func TestSmoke_SkillResolution(t *testing.T) {
	dir := newWorkspace(t)
	initWorkspace(t, dir, "test-ws")

	// Create a local skill.
	skillDir := filepath.Join(dir, ".toc", "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# test-skill\nA test skill.\n"), 0644)

	// Create agent that references the skill.
	agentDir := filepath.Join(dir, ".toc", "agents", "skilled")
	os.MkdirAll(agentDir, 0755)

	cfg := `runtime: claude-code
name: skilled
model: sonnet
skills:
  - test-skill
`
	os.WriteFile(filepath.Join(agentDir, "oc-agent.yaml"), []byte(cfg), 0644)
	os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("# skilled\n"), 0644)

	out := runToc(t, dir, "agent", "spawn", "skilled")

	// Should report skills resolved.
	if !strings.Contains(out, "Skills:") || !strings.Contains(out, "1/1") {
		t.Errorf("expected '1/1 resolved' in output, got: %s", out)
	}

	// Verify skill was copied to session workspace.
	wsPath := extractWorkspacePath(out)
	if wsPath == "" {
		t.Fatal("could not extract workspace path")
	}
	skillPath := filepath.Join(wsPath, ".claude", "skills", "test-skill", "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Errorf("expected skill to be copied to session workspace at %s", skillPath)
	}
}

// --- Helpers ---

// extractSessionID pulls the session ID from toc runtime spawn output.
func extractSessionID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Session ID:") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	return ""
}

// extractWorkspacePath pulls the workspace path from toc agent spawn output.
func extractWorkspacePath(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Workspace:") {
			// The path is the last field on the line.
			trimmed := strings.TrimSpace(line)
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	return ""
}

// waitForSubAgent polls the session workspace for completion markers.
func waitForSubAgent(t *testing.T, workspaceDir, sessionID string, timeout time.Duration) {
	t.Helper()

	// Read sessions.yaml to find the workspace path.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sessData, err := os.ReadFile(filepath.Join(workspaceDir, ".toc/sessions.yaml"))
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Find the workspace_path for this session.
		wsPath := ""
		lines := strings.Split(string(sessData), "\n")
		for i, line := range lines {
			if strings.Contains(line, sessionID) {
				// Look for workspace_path in nearby lines.
				for j := i; j < len(lines) && j < i+10; j++ {
					if strings.Contains(lines[j], "workspace_path:") {
						wsPath = strings.TrimSpace(strings.SplitN(lines[j], ":", 2)[1])
						break
					}
				}
				break
			}
		}

		if wsPath == "" {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Check for completion marker (toc-output.txt).
		if _, err := os.Stat(filepath.Join(wsPath, "toc-output.txt")); err == nil {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("sub-agent %s did not complete within %v", sessionID, timeout)
}
