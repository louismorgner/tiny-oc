package spawn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDetachedScript_PIDTracking(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test prompt"), 0644)

	script := buildDetachedScript(detachedOpts{
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

	script := buildDetachedScript(detachedOpts{
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

	script := buildDetachedScript(detachedOpts{
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

	script := buildDetachedScript(detachedOpts{
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

	script := buildDetachedScript(detachedOpts{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess", OutputPath: filepath.Join(dir, "toc-output.txt"),
	}, promptPath)

	if strings.Contains(script, "--model") {
		t.Error("script should not contain --model when model is empty")
	}
}

func TestBuildDetachedScript_Resume_UsesContinueFlag(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("resume prompt"), 0644)

	script := buildDetachedScript(detachedOpts{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess-789", OutputPath: filepath.Join(dir, "toc-output.txt"),
		Resume: true,
	}, promptPath)

	if !strings.Contains(script, "--continue") {
		t.Error("resume script should use --continue flag")
	}
	if !strings.Contains(script, "echo $$ >") {
		t.Error("resume script should write PID file")
	}
	if !strings.Contains(script, "TOC_EXIT=$?") {
		t.Error("resume script should capture exit code")
	}
}

func TestBuildDetachedScript_NoResume_NoContinueFlag(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)

	script := buildDetachedScript(detachedOpts{
		Dir: dir, Workspace: "/ws", AgentName: "agent",
		SessionID: "sess", OutputPath: filepath.Join(dir, "toc-output.txt"),
		Resume: false,
	}, promptPath)

	if strings.Contains(script, "--continue") {
		t.Error("non-resume script should not contain --continue flag")
	}
}

func TestBuildDetachedScript_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	os.WriteFile(promptPath, []byte("test"), 0644)
	outputPath := filepath.Join(dir, "toc-output.txt")

	script := buildDetachedScript(detachedOpts{
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
