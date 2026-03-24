package spawn

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/session"
	tocsync "github.com/tiny-oc/toc/internal/sync"
	"github.com/tiny-oc/toc/internal/ui"
)

const sessionsDir = "/tmp/toc-sessions"

// SpawnResult contains metadata from a completed spawn for audit logging.
type SpawnResult struct {
	SessionID   string
	SyncedFiles int
}

func SpawnSession(cfg *agent.AgentConfig) (*SpawnResult, error) {
	sessionID := uuid.New().String()
	timestamp := time.Now().Unix()
	workDir := filepath.Join(sessionsDir, fmt.Sprintf("%s-%d", cfg.Name, timestamp))

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	srcDir, err := filepath.Abs(agent.Dir(cfg.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent directory: %w", err)
	}

	if err := copyDir(srcDir, workDir); err != nil {
		return nil, fmt.Errorf("failed to copy agent template: %w", err)
	}

	if len(cfg.Context) > 0 {
		if err := setupContextHooks(workDir, srcDir, cfg.Context); err != nil {
			return nil, fmt.Errorf("failed to setup context sync hooks: %w", err)
		}
	}

	if err := session.Add(session.Session{
		ID:            sessionID,
		Agent:         cfg.Name,
		CreatedAt:     time.Now(),
		WorkspacePath: workDir,
	}); err != nil {
		return nil, fmt.Errorf("failed to track session: %w", err)
	}

	fmt.Println()
	ui.Info("Agent: %s", ui.Bold(cfg.Name))
	ui.Info("Model: %s", ui.Bold(cfg.Model))
	ui.Info("Session: %s", ui.Cyan(sessionID))
	ui.Info("Workspace: %s", ui.Dim(workDir))
	if len(cfg.Context) > 0 {
		ui.Info("Context sync: %s", ui.Dim(fmt.Sprintf("%d pattern(s)", len(cfg.Context))))
	}
	fmt.Println()

	_ = launchClaude(workDir, cfg.Model, sessionID, false)

	// Post-session sync: copy matching files back to agent template
	var syncedFiles int
	if len(cfg.Context) > 0 {
		syncedFiles = runPostSessionSync(workDir, srcDir, cfg.Context)
	}

	printResumeCommand(cfg.Name, sessionID)

	return &SpawnResult{SessionID: sessionID, SyncedFiles: syncedFiles}, nil
}

func ResumeSession(s *session.Session) (*SpawnResult, error) {
	if _, err := os.Stat(s.WorkspacePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("session workspace no longer exists: %s", s.WorkspacePath)
	}

	cfg, err := agent.Load(s.Agent)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent config: %w", err)
	}

	srcDir, err := filepath.Abs(agent.Dir(s.Agent))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent directory: %w", err)
	}

	// Re-setup hooks in case they were cleaned up
	if len(cfg.Context) > 0 {
		if err := setupContextHooks(s.WorkspacePath, srcDir, cfg.Context); err != nil {
			return nil, fmt.Errorf("failed to setup context sync hooks: %w", err)
		}
	}

	fmt.Println()
	ui.Info("Resuming agent: %s", ui.Bold(s.Agent))
	ui.Info("Session: %s", ui.Cyan(s.ID))
	ui.Info("Workspace: %s", ui.Dim(s.WorkspacePath))
	fmt.Println()

	_ = launchClaude(s.WorkspacePath, cfg.Model, s.ID, true)

	var syncedFiles int
	if len(cfg.Context) > 0 {
		syncedFiles = runPostSessionSync(s.WorkspacePath, srcDir, cfg.Context)
	}

	printResumeCommand(s.Agent, s.ID)

	return &SpawnResult{SessionID: s.ID, SyncedFiles: syncedFiles}, nil
}

func setupContextHooks(workDir, agentDir string, patterns []string) error {
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return err
	}

	// Write sync script
	scriptPath := filepath.Join(claudeDir, "toc-sync.sh")
	script := tocsync.SyncScript(workDir, agentDir, patterns)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}

	// Write settings.json with hook config
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Merge with existing settings if present
	settings, err := tocsync.HookSettings(scriptPath)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, settings, 0644)
}

func runPostSessionSync(workDir, agentDir string, patterns []string) int {
	count, err := tocsync.SyncBack(workDir, agentDir, patterns)
	if err != nil {
		ui.Warn("Context sync error: %s", err)
		return 0
	}
	if count > 0 {
		ui.Success("Synced %d context file(s) back to agent template", count)
	}
	return count
}

func printResumeCommand(agentName, sessionID string) {
	fmt.Println()
	fmt.Printf("  %s\n", ui.Dim("───────────────────────────────────────"))
	ui.Info("Session ended. Resume with:")
	fmt.Println()
	ui.Command(fmt.Sprintf("toc agent spawn %s --resume %s", agentName, sessionID))
	fmt.Println()
}

func launchClaude(dir, model, sessionID string, resume bool) error {
	args := []string{}
	if model != "" {
		args = append(args, "--model", model)
	}
	if resume {
		args = append(args, "--resume", sessionID)
	} else {
		args = append(args, "--session-id", sessionID)
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil // normal exit (ctrl-c, quit, etc.)
		}
		return fmt.Errorf("failed to launch claude: %w", err)
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
