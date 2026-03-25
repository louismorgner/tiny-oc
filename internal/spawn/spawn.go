package spawn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/fileutil"
	"github.com/tiny-oc/toc/internal/gitutil"
	"github.com/tiny-oc/toc/internal/registry"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/skill"
	tocsync "github.com/tiny-oc/toc/internal/sync"
	"github.com/tiny-oc/toc/internal/ui"
)

// SpawnResult contains metadata from a completed spawn for audit logging.
type SpawnResult struct {
	SessionID    string
	SyncedFiles  int
	FailedSkills []string
}

func SpawnSession(cfg *agent.AgentConfig) (*SpawnResult, error) {
	sessionID := uuid.New().String()

	// Use os.MkdirTemp for unpredictable, safe session directories
	baseDir := filepath.Join(os.TempDir(), "toc-sessions")
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create sessions base directory: %w", err)
	}
	workDir, err := os.MkdirTemp(baseDir, cfg.Name+"-")
	if err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	srcDir, err := filepath.Abs(agent.Dir(cfg.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent directory: %w", err)
	}

	if err := fileutil.CopyDir(srcDir, workDir); err != nil {
		return nil, fmt.Errorf("failed to copy agent template: %w", err)
	}

	// Convert agent.md (+ compose files) into CLAUDE.md so Claude Code picks it up as instructions
	if err := provisionClaudeMD(workDir, cfg, sessionID); err != nil {
		return nil, fmt.Errorf("failed to provision CLAUDE.md: %w", err)
	}

	// Resolve skills into session .claude/skills/
	var failedSkills []string
	if len(cfg.Skills) > 0 {
		failedSkills = resolveSkills(workDir, cfg.Skills)
	}

	if len(cfg.Context) > 0 {
		if err := setupContextHooks(workDir, srcDir, cfg.Context, cfg.OnEnd); err != nil {
			return nil, fmt.Errorf("failed to setup context sync hooks: %w", err)
		}
	} else if cfg.OnEnd != "" {
		if err := setupOnEndHook(workDir, cfg.OnEnd); err != nil {
			return nil, fmt.Errorf("failed to setup on_end hook: %w", err)
		}
	}

	if err := session.Add(session.Session{
		ID:            sessionID,
		Agent:         cfg.Name,
		CreatedAt:     time.Now(),
		WorkspacePath: workDir,
		Status:        session.StatusActive,
	}); err != nil {
		return nil, fmt.Errorf("failed to track session: %w", err)
	}

	fmt.Println()
	ui.Info("Agent: %s", ui.Bold(cfg.Name))
	ui.Info("Model: %s", ui.Bold(cfg.Model))
	ui.Info("Session: %s", ui.Cyan(sessionID))
	ui.Info("Workspace: %s", ui.Dim(workDir))
	if len(cfg.Skills) > 0 {
		resolved := len(cfg.Skills) - len(failedSkills)
		ui.Info("Skills: %s", ui.Dim(fmt.Sprintf("%d/%d resolved", resolved, len(cfg.Skills))))
	}
	if len(cfg.Context) > 0 {
		ui.Info("Context sync: %s", ui.Dim(fmt.Sprintf("%d pattern(s)", len(cfg.Context))))
	}
	if cfg.OnEnd != "" {
		ui.Info("On end: %s", ui.Dim("session end hook enabled"))
	}
	fmt.Println()

	_ = launchClaude(workDir, cfg.Model, sessionID, cfg.Name, false)
	_ = session.UpdateStatus(sessionID, session.StatusCompleted)

	// Post-session sync: copy matching files back to agent template
	var syncedFiles int
	if len(cfg.Context) > 0 {
		syncedFiles = runPostSessionSync(workDir, srcDir, cfg.Context)
	}

	printFailedSkills(failedSkills)
	printResumeCommand(cfg.Name, sessionID)

	return &SpawnResult{SessionID: sessionID, SyncedFiles: syncedFiles, FailedSkills: failedSkills}, nil
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

	// Re-resolve skills in case they were cleaned up or updated
	var failedSkills []string
	if len(cfg.Skills) > 0 {
		failedSkills = resolveSkills(s.WorkspacePath, cfg.Skills)
	}

	// Re-setup hooks in case they were cleaned up
	if len(cfg.Context) > 0 {
		if err := setupContextHooks(s.WorkspacePath, srcDir, cfg.Context, cfg.OnEnd); err != nil {
			return nil, fmt.Errorf("failed to setup context sync hooks: %w", err)
		}
	} else if cfg.OnEnd != "" {
		if err := setupOnEndHook(s.WorkspacePath, cfg.OnEnd); err != nil {
			return nil, fmt.Errorf("failed to setup on_end hook: %w", err)
		}
	}

	fmt.Println()
	ui.Info("Resuming agent: %s", ui.Bold(s.Agent))
	ui.Info("Session: %s", ui.Cyan(s.ID))
	ui.Info("Workspace: %s", ui.Dim(s.WorkspacePath))
	if len(cfg.Skills) > 0 {
		resolved := len(cfg.Skills) - len(failedSkills)
		ui.Info("Skills: %s", ui.Dim(fmt.Sprintf("%d/%d resolved", resolved, len(cfg.Skills))))
	}
	fmt.Println()

	_ = session.UpdateStatus(s.ID, session.StatusActive)
	_ = launchClaude(s.WorkspacePath, cfg.Model, s.ID, s.Agent, true)
	_ = session.UpdateStatus(s.ID, session.StatusCompleted)

	var syncedFiles int
	if len(cfg.Context) > 0 {
		syncedFiles = runPostSessionSync(s.WorkspacePath, srcDir, cfg.Context)
	}

	printFailedSkills(failedSkills)
	printResumeCommand(s.Agent, s.ID)

	return &SpawnResult{SessionID: s.ID, SyncedFiles: syncedFiles, FailedSkills: failedSkills}, nil
}

// SubSpawnOpts contains options for spawning a sub-agent session.
type SubSpawnOpts struct {
	ParentSessionID string
	Prompt          string
	WorkspaceDir    string // absolute path to the toc workspace
}

// SpawnSubSession spawns a non-interactive sub-agent session in the background.
// The sub-agent runs with `claude --print` and output is captured to toc-output.txt.
func SpawnSubSession(cfg *agent.AgentConfig, opts SubSpawnOpts) (*SpawnResult, error) {
	sessionID := uuid.New().String()

	// Use os.MkdirTemp for unpredictable, safe session directories
	baseDir := filepath.Join(os.TempDir(), "toc-sessions")
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create sessions base directory: %w", err)
	}
	workDir, err := os.MkdirTemp(baseDir, cfg.Name+"-")
	if err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	// Use workspace-relative agent dir
	agentDir := filepath.Join(opts.WorkspaceDir, ".toc", "agents", cfg.Name)
	if err := fileutil.CopyDir(agentDir, workDir); err != nil {
		return nil, fmt.Errorf("failed to copy agent template: %w", err)
	}

	if err := provisionClaudeMD(workDir, cfg, sessionID); err != nil {
		return nil, fmt.Errorf("failed to provision CLAUDE.md: %w", err)
	}

	var failedSkills []string
	if len(cfg.Skills) > 0 {
		failedSkills = resolveSkills(workDir, cfg.Skills)
	}

	if err := session.AddInWorkspace(opts.WorkspaceDir, session.Session{
		ID:              sessionID,
		Agent:           cfg.Name,
		CreatedAt:       time.Now(),
		WorkspacePath:   workDir,
		Status:          session.StatusActive,
		ParentSessionID: opts.ParentSessionID,
		Prompt:          opts.Prompt,
	}); err != nil {
		return nil, fmt.Errorf("failed to track session: %w", err)
	}

	// Launch claude as a detached background process so it survives after toc exits
	outputPath := filepath.Join(workDir, "toc-output.txt")
	if err := launchClaudeDetached(workDir, cfg.Model, opts.Prompt, opts.WorkspaceDir, cfg.Name, sessionID, outputPath); err != nil {
		return nil, fmt.Errorf("failed to launch sub-agent: %w", err)
	}

	return &SpawnResult{SessionID: sessionID, FailedSkills: failedSkills}, nil
}

// launchClaudeDetached starts claude in a detached process via a wrapper script
// so the sub-agent outlives the parent toc process.
func launchClaudeDetached(dir, model, prompt, workspace, agentName, sessionID, outputPath string) error {
	// Write the prompt to a file to avoid shell injection via interpolation.
	promptPath := filepath.Join(dir, "toc-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		return err
	}

	// Build claude command that reads prompt from the file.
	args := fmt.Sprintf("claude --dangerously-skip-permissions --print -p \"$(cat %q)\"", promptPath)
	if model != "" {
		args += fmt.Sprintf(" --model %s", model)
	}

	// Write a wrapper script that runs claude and captures output.
	// The existence of toc-output.txt signals completion (checked by ResolvedStatus).
	scriptContent := fmt.Sprintf(`#!/bin/sh
cd %q
export TOC_WORKSPACE=%q
export TOC_AGENT=%q
export TOC_SESSION_ID=%q
%s > %q 2>&1
`, dir, workspace, agentName, sessionID, args, outputPath)

	scriptPath := filepath.Join(dir, "toc-run.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return err
	}

	cmd := exec.Command("sh", scriptPath)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// Start the process detached
	if err := cmd.Start(); err != nil {
		return err
	}

	// Release the process so it's not waited on
	return cmd.Process.Release()
}

func setupContextHooks(workDir, agentDir string, patterns []string, onEnd string) error {
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

	// Write settings.json with hook config (includes SessionEnd if on_end is set)
	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings, err := tocsync.HookSettings(scriptPath, onEnd)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, settings, 0644)
}

func setupOnEndHook(workDir, onEnd string) error {
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return err
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings, err := tocsync.OnEndHookSettings(onEnd)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, settings, 0644)
}

func runPostSessionSync(workDir, agentDir string, patterns []string) int {
	synced, err := tocsync.SyncBack(workDir, agentDir, patterns)
	if err != nil {
		ui.Warn("Context sync error: %s", err)
		return 0
	}
	if len(synced) > 0 {
		ui.Success("Synced %d context file(s) back to agent template", len(synced))
		for _, f := range synced {
			fmt.Printf("    %s %s\n", ui.Dim("▪"), ui.Dim(f))
		}
	}
	return len(synced)
}

func printResumeCommand(agentName, sessionID string) {
	fmt.Println()
	fmt.Printf("  %s\n", ui.Dim("───────────────────────────────────────"))
	ui.Info("Session ended. Resume with:")
	fmt.Println()
	ui.Command(fmt.Sprintf("toc agent spawn %s --resume %s", agentName, sessionID))
	fmt.Println()
}

func launchClaude(dir, model, sessionID, agentName string, resume bool) error {
	args := []string{"--dangerously-skip-permissions"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if resume {
		args = append(args, "--resume", sessionID)
	} else {
		args = append(args, "--session-id", sessionID)
	}

	workspace, _ := filepath.Abs(".")

	cmd := exec.Command("claude", args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"TOC_WORKSPACE="+workspace,
		"TOC_AGENT="+agentName,
		"TOC_SESSION_ID="+sessionID,
	)

	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil // normal exit (ctrl-c, quit, etc.)
		}
		return fmt.Errorf("failed to launch claude: %w", err)
	}
	return nil
}

// resolveSkills resolves all skill references and copies them into the session's
// .claude/skills/ directory. Returns a list of skill entries that failed to resolve.
func resolveSkills(workDir string, skills []string) []string {
	skillsTarget := filepath.Join(workDir, ".claude", "skills")
	if err := os.MkdirAll(skillsTarget, 0755); err != nil {
		ui.Warn("Failed to create skills directory: %s", err)
		return skills
	}

	var failed []string
	for _, entry := range skills {
		if skill.IsURL(entry) {
			if err := resolveURLSkill(entry, skillsTarget); err != nil {
				ui.Warn("Skill '%s': %s", entry, err)
				failed = append(failed, entry)
			}
		} else {
			if err := resolveNamedSkill(entry, skillsTarget); err != nil {
				ui.Warn("Skill '%s': %s", entry, err)
				failed = append(failed, entry)
			}
		}
	}
	return failed
}

// resolveNamedSkill resolves a skill by name:
// 1. Local .toc/skills/<name>/
// 2. URL reference in .toc/skills.yaml
// 3. Remote toc registry (registry/skills/<name>/)
func resolveNamedSkill(name string, targetDir string) error {
	// Try local skill first
	srcDir := skill.Dir(name)
	if _, err := os.Stat(srcDir); err == nil {
		return fileutil.CopyDir(srcDir, filepath.Join(targetDir, name))
	}

	// Try URL registry
	ref, err := skill.FindRef(name)
	if err == nil {
		return cloneSkillToTarget(ref.URL, targetDir)
	}

	// Fall back to remote toc registry
	meta, err := registry.InstallSkillTo(name, filepath.Join(targetDir, name))
	if err != nil {
		return fmt.Errorf("not found locally, in URL refs, or in registry")
	}
	_ = meta
	return nil
}

// resolveURLSkill resolves a skill directly from a URL.
func resolveURLSkill(url string, targetDir string) error {
	return cloneSkillToTarget(url, targetDir)
}

// cloneSkillToTarget clones a git repo, finds the SKILL.md, and copies the skill
// directory into the target.
func cloneSkillToTarget(url string, targetDir string) error {
	tmpDir, err := os.MkdirTemp("", "toc-skill-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := gitutil.SafeClone(url, tmpDir); err != nil {
		return err
	}

	skillDir, err := skill.FindSkillMDInDir(tmpDir)
	if err != nil {
		return err
	}

	meta, err := skill.ValidateSkillDir(skillDir)
	if err != nil {
		return err
	}

	return fileutil.CopyDir(skillDir, filepath.Join(targetDir, meta.Name))
}

func printFailedSkills(failed []string) {
	if len(failed) == 0 {
		return
	}
	fmt.Println()
	ui.Warn("Some skills failed to resolve:")
	for _, s := range failed {
		fmt.Printf("  %s %s\n", ui.Red("✗"), s)
	}
	ui.Info("Consider removing or updating these skill references.")
}

// provisionClaudeMD builds CLAUDE.md from agent.md + any compose files,
// then applies template variables (agent name, session ID, date).
func provisionClaudeMD(workDir string, cfg *agent.AgentConfig, sessionID string) error {
	agentMD := filepath.Join(workDir, "agent.md")
	claudeMD := filepath.Join(workDir, "CLAUDE.md")

	// Start with agent.md content (always first)
	var parts []string
	if data, err := os.ReadFile(agentMD); err == nil {
		parts = append(parts, strings.TrimSpace(string(data)))
		os.Remove(agentMD)
	}

	// Append compose files in order
	for _, file := range cfg.Compose {
		path := filepath.Join(workDir, file)
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, strings.TrimSpace(string(data)))
		}
	}

	if len(parts) == 0 {
		return nil // nothing to provision
	}

	content := strings.Join(parts, "\n\n---\n\n")

	// Apply template variables
	now := time.Now()
	replacer := strings.NewReplacer(
		"{{.AgentName}}", cfg.Name,
		"{{.SessionID}}", sessionID,
		"{{.Date}}", now.Format("2006-01-02"),
		"{{.Model}}", cfg.Model,
	)
	content = replacer.Replace(content)

	return os.WriteFile(claudeMD, []byte(content+"\n"), 0644)
}

