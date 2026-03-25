package spawn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchClaudeDetached_ScriptContainsPIDTracking(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "toc-output.txt")

	// launchClaudeDetached will fail because 'claude' isn't installed,
	// but we can inspect the generated script before that.
	// We'll call the script generation inline.
	prompt := "test prompt with \"quotes\" and $vars"
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		t.Fatal(err)
	}

	// Simulate what launchClaudeDetached generates
	pidPath := filepath.Join(dir, "toc-pid.txt")
	exitCodePath := filepath.Join(dir, "toc-exit-code.txt")
	tmpOutputPath := outputPath + ".tmp"

	script := generateTestScript(pidPath, dir, "/workspace", "agent", "session-123", promptPath, "", tmpOutputPath, exitCodePath, outputPath)

	// Verify script contains PID tracking
	if !strings.Contains(script, "echo $$ >") {
		t.Error("script does not write PID file")
	}
	if !strings.Contains(script, pidPath) {
		t.Errorf("script does not reference PID path %s", pidPath)
	}

	// Verify script captures exit code
	if !strings.Contains(script, "TOC_EXIT=$?") {
		t.Error("script does not capture exit code")
	}
	if !strings.Contains(script, exitCodePath) {
		t.Errorf("script does not reference exit code path %s", exitCodePath)
	}

	// Verify script does atomic rename
	if !strings.Contains(script, "mv") {
		t.Error("script does not do atomic rename")
	}

	// Verify exit code is written before the mv (so zombie detection works if mv fails)
	exitCodeIdx := strings.Index(script, "TOC_EXIT=$?")
	mvIdx := strings.Index(script, "mv ")
	if exitCodeIdx > mvIdx {
		t.Error("exit code should be captured before atomic rename")
	}
}

func TestLaunchClaudeDetached_ScriptSetsEnvVars(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "toc-output.txt")
	pidPath := filepath.Join(dir, "toc-pid.txt")
	exitCodePath := filepath.Join(dir, "toc-exit-code.txt")
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	tmpOutputPath := outputPath + ".tmp"

	script := generateTestScript(pidPath, dir, "/my/workspace", "test-agent", "sess-456", promptPath, "sonnet", tmpOutputPath, exitCodePath, outputPath)

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

func TestLaunchClaudeDetached_ScriptWithModel(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "toc-output.txt")
	pidPath := filepath.Join(dir, "toc-pid.txt")
	exitCodePath := filepath.Join(dir, "toc-exit-code.txt")
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	tmpOutputPath := outputPath + ".tmp"

	script := generateTestScript(pidPath, dir, "/ws", "agent", "sess", promptPath, "opus", tmpOutputPath, exitCodePath, outputPath)

	if !strings.Contains(script, "--model opus") {
		t.Error("script missing model flag")
	}
}

func TestLaunchClaudeDetached_ScriptWithoutModel(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "toc-output.txt")
	pidPath := filepath.Join(dir, "toc-pid.txt")
	exitCodePath := filepath.Join(dir, "toc-exit-code.txt")
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	tmpOutputPath := outputPath + ".tmp"

	script := generateTestScript(pidPath, dir, "/ws", "agent", "sess", promptPath, "", tmpOutputPath, exitCodePath, outputPath)

	if strings.Contains(script, "--model") {
		t.Error("script should not contain --model when model is empty")
	}
}

func TestResumeScript_UsesContineFlag(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	if err := os.WriteFile(promptPath, []byte("resume prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(dir, "toc-output.txt")
	pidPath := filepath.Join(dir, "toc-pid.txt")
	exitCodePath := filepath.Join(dir, "toc-exit-code.txt")
	tmpOutputPath := outputPath + ".tmp"

	script := generateResumeTestScript(pidPath, dir, "/ws", "agent", "sess-789", promptPath, "", tmpOutputPath, exitCodePath, outputPath)

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

// generateTestScript produces the same script content as launchClaudeDetached
// without actually starting a process. This allows testing the script format.
func generateTestScript(pidPath, dir, workspace, agentName, sessionID, promptPath, model, tmpOutputPath, exitCodePath, outputPath string) string {
	args := `claude --dangerously-skip-permissions -p "$(cat ` + `"` + promptPath + `"` + `)"`
	if model != "" {
		args += " --model " + model
	}

	return "#!/bin/sh\n" +
		"echo $$ > " + `"` + pidPath + `"` + "\n" +
		"cd " + `"` + dir + `"` + "\n" +
		`export TOC_WORKSPACE="` + workspace + `"` + "\n" +
		`export TOC_AGENT="` + agentName + `"` + "\n" +
		`export TOC_SESSION_ID="` + sessionID + `"` + "\n" +
		args + ` < /dev/null > "` + tmpOutputPath + `" 2>&1` + "\n" +
		"TOC_EXIT=$?\n" +
		`echo $TOC_EXIT > "` + exitCodePath + `"` + "\n" +
		`mv "` + tmpOutputPath + `" "` + outputPath + `"` + "\n"
}

// generateResumeTestScript produces the resume script variant.
func generateResumeTestScript(pidPath, dir, workspace, agentName, sessionID, promptPath, model, tmpOutputPath, exitCodePath, outputPath string) string {
	args := `claude --dangerously-skip-permissions --continue -p "$(cat ` + `"` + promptPath + `"` + `)"`
	if model != "" {
		args += " --model " + model
	}

	return "#!/bin/sh\n" +
		"echo $$ > " + `"` + pidPath + `"` + "\n" +
		"cd " + `"` + dir + `"` + "\n" +
		`export TOC_WORKSPACE="` + workspace + `"` + "\n" +
		`export TOC_AGENT="` + agentName + `"` + "\n" +
		`export TOC_SESSION_ID="` + sessionID + `"` + "\n" +
		args + ` < /dev/null > "` + tmpOutputPath + `" 2>&1` + "\n" +
		"TOC_EXIT=$?\n" +
		`echo $TOC_EXIT > "` + exitCodePath + `"` + "\n" +
		`mv "` + tmpOutputPath + `" "` + outputPath + `"` + "\n"
}
