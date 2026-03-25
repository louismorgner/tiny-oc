package spawn

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/fileutil"
	"github.com/tiny-oc/toc/internal/gitutil"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/naming"
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

	if err := setupHooks(workDir, srcDir, cfg); err != nil {
		return nil, fmt.Errorf("failed to setup hooks: %w", err)
	}

	// Write resolved permission manifest for integration gateway enforcement
	if err := writePermissionManifest(sessionID, cfg); err != nil {
		return nil, fmt.Errorf("failed to write permission manifest: %w", err)
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
	if cfg.Perms != nil {
		ui.Info("Permissions: %s", ui.Dim("enforced via hooks"))
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
	if err := setupHooks(s.WorkspacePath, srcDir, cfg); err != nil {
		return nil, fmt.Errorf("failed to setup hooks: %w", err)
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

	// Write resolved permission manifest for sub-agent
	if err := writePermissionManifestInWorkspace(opts.WorkspaceDir, sessionID, cfg); err != nil {
		return nil, fmt.Errorf("failed to write permission manifest: %w", err)
	}

	if err := session.AddInWorkspace(opts.WorkspaceDir, session.Session{
		ID:              sessionID,
		Name:            naming.FromPrompt(opts.Prompt),
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

// SubResumeOpts contains options for resuming a sub-agent session.
type SubResumeOpts struct {
	ParentSessionID string
	Prompt          string // optional additional context for the resumed session
	WorkspaceDir    string
}

// ResumeSubSession resumes a failed, zombie, or cancelled sub-agent session.
// The resumed session reuses the existing session directory and conversation history.
func ResumeSubSession(s *session.Session, opts SubResumeOpts) (*SpawnResult, error) {
	if _, err := os.Stat(s.WorkspacePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("session workspace no longer exists: %s", s.WorkspacePath)
	}

	status := s.ResolvedStatus()
	if status == "active" {
		return nil, fmt.Errorf("session '%s' is still active — cannot resume an active session", s.ID)
	}
	if status == "stale" {
		return nil, fmt.Errorf("session '%s' workspace no longer exists", s.ID)
	}

	agentDir := filepath.Join(opts.WorkspaceDir, ".toc", "agents", s.Agent)
	cfg, err := agent.LoadFrom(agentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent config: %w", err)
	}

	// Re-provision CLAUDE.md in case the template has changed
	if err := provisionClaudeMD(s.WorkspacePath, cfg, s.ID); err != nil {
		return nil, fmt.Errorf("failed to re-provision CLAUDE.md: %w", err)
	}

	// Re-resolve skills in case they were cleaned up
	var failedSkills []string
	if len(cfg.Skills) > 0 {
		failedSkills = resolveSkills(s.WorkspacePath, cfg.Skills)
	}

	// Re-setup hooks
	if err := setupHooks(s.WorkspacePath, agentDir, cfg); err != nil {
		return nil, fmt.Errorf("failed to setup hooks: %w", err)
	}

	// Clean up previous run markers so ResolvedStatus tracks the new run
	for _, marker := range []string{"toc-output.txt", "toc-output.txt.tmp", "toc-exit-code.txt", "toc-pid.txt", "toc-cancelled.txt"} {
		os.Remove(filepath.Join(s.WorkspacePath, marker))
	}

	// Update session status back to active
	_ = session.UpdateStatusInWorkspace(opts.WorkspaceDir, s.ID, session.StatusActive)

	// Build the prompt for the resumed session
	resumePrompt := "You are resuming a previous session that was interrupted. Check your git status and continue where you left off."
	if opts.Prompt != "" {
		resumePrompt = opts.Prompt
	}

	// Launch claude detached with --continue to resume the previous conversation
	outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
	if err := launchDetached(detachedOpts{
		Dir: s.WorkspacePath, Model: cfg.Model, Prompt: resumePrompt,
		Workspace: opts.WorkspaceDir, AgentName: s.Agent, SessionID: s.ID,
		OutputPath: outputPath, Resume: true,
	}); err != nil {
		return nil, fmt.Errorf("failed to launch resumed sub-agent: %w", err)
	}

	return &SpawnResult{SessionID: s.ID, FailedSkills: failedSkills}, nil
}

// detachedOpts contains options for launching a detached claude process.
type detachedOpts struct {
	Dir, Model, Prompt, Workspace, AgentName, SessionID, OutputPath string
	Resume                                                          bool
}

// launchClaudeDetached starts claude in a detached process via a wrapper script
// so the sub-agent outlives the parent toc process.
func launchClaudeDetached(dir, model, prompt, workspace, agentName, sessionID, outputPath string) error {
	return launchDetached(detachedOpts{
		Dir: dir, Model: model, Prompt: prompt, Workspace: workspace,
		AgentName: agentName, SessionID: sessionID, OutputPath: outputPath,
	})
}

// launchDetached is the shared implementation for starting a detached claude process.
// When Resume is true, it uses --continue to resume a previous conversation.
func launchDetached(opts detachedOpts) error {
	// Write the prompt to a file to avoid shell injection via interpolation.
	promptPath := filepath.Join(opts.Dir, "toc-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0644); err != nil {
		return err
	}

	scriptContent := buildDetachedScript(opts, promptPath)

	scriptPath := filepath.Join(opts.Dir, "toc-run.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return err
	}

	cmd := exec.Command("sh", scriptPath)
	cmd.Dir = opts.Dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}

// buildDetachedScript generates the shell script content for a detached claude process.
// Separated from launchDetached so it can be tested without starting a process.
func buildDetachedScript(opts detachedOpts, promptPath string) string {
	// Build claude command that reads prompt from the file.
	args := "claude --dangerously-skip-permissions"
	if opts.Resume {
		args += " --continue"
	}
	args += fmt.Sprintf(" -p \"$(cat %q)\"", promptPath)
	if opts.Model != "" {
		args += fmt.Sprintf(" --model %s", opts.Model)
	}

	// We write to a .tmp file first, then atomically rename to the final path.
	// This prevents a race: shell `>` creates the file immediately (empty),
	// but ResolvedStatus checks for toc-output.txt existence as the completion signal.
	tmpOutputPath := opts.OutputPath + ".tmp"
	pidPath := filepath.Join(opts.Dir, "toc-pid.txt")
	exitCodePath := filepath.Join(opts.Dir, "toc-exit-code.txt")

	return fmt.Sprintf(`#!/bin/sh
echo $$ > %q
cd %q
export TOC_WORKSPACE=%q
export TOC_AGENT=%q
export TOC_SESSION_ID=%q
%s < /dev/null > %q 2>&1
TOC_EXIT=$?
echo $TOC_EXIT > %q
mv %q %q
`, pidPath, opts.Dir, opts.Workspace, opts.AgentName, opts.SessionID, args, tmpOutputPath, exitCodePath, tmpOutputPath, opts.OutputPath)
}

// setupHooks generates .claude/settings.json with all hooks: context sync,
// on_end, and permission enforcement. This is the single entry point for hook setup.
func setupHooks(workDir, agentDir string, cfg *agent.AgentConfig) error {
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return err
	}

	var syncScriptPath string
	if len(cfg.Context) > 0 {
		syncScriptPath = filepath.Join(claudeDir, "toc-sync.sh")
		script := tocsync.SyncScript(workDir, agentDir, cfg.Context)
		if err := os.WriteFile(syncScriptPath, []byte(script), 0755); err != nil {
			return err
		}
	}

	var permScriptPath string
	if cfg.Perms != nil {
		perms := cfg.EffectivePermissions()
		permScriptPath = filepath.Join(claudeDir, "toc-permissions.sh")
		script := tocsync.PermissionScript(perms, cfg.Name)
		if err := os.WriteFile(permScriptPath, []byte(script), 0755); err != nil {
			return err
		}
	}

	// Only write settings if we have hooks to configure
	if syncScriptPath == "" && cfg.OnEnd == "" && permScriptPath == "" {
		return nil
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings, err := tocsync.HookSettingsWithPermissions(syncScriptPath, cfg.OnEnd, permScriptPath)
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

// writePermissionManifest writes the resolved integration permissions to
// .toc/sessions/<id>/permissions.json. This manifest is immutable after spawn —
// the running session keeps its original permissions even if oc-agent.yaml changes.
func writePermissionManifest(sessionID string, cfg *agent.AgentConfig) error {
	perms := cfg.EffectivePermissions()
	if len(perms.Integrations) == 0 {
		return nil
	}

	manifest := integration.PermissionManifest{
		SessionID:    sessionID,
		Agent:        cfg.Name,
		Integrations: perms.Integrations,
	}

	dir := filepath.Join(config.SessionsDir(), sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create session permissions directory: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "permissions.json"), data, 0600)
}

// writePermissionManifestInWorkspace writes the permission manifest using an
// explicit workspace path. Used for sub-agent spawning.
func writePermissionManifestInWorkspace(workspace, sessionID string, cfg *agent.AgentConfig) error {
	perms := cfg.EffectivePermissions()
	if len(perms.Integrations) == 0 {
		return nil
	}

	manifest := integration.PermissionManifest{
		SessionID:    sessionID,
		Agent:        cfg.Name,
		Integrations: perms.Integrations,
	}

	dir := filepath.Join(workspace, ".toc", "sessions", sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create session permissions directory: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "permissions.json"), data, 0600)
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
