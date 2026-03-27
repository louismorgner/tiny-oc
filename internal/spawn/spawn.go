package spawn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/fileutil"
	"github.com/tiny-oc/toc/internal/gitutil"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/naming"
	"github.com/tiny-oc/toc/internal/registry"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

// SpawnResult contains metadata from a completed spawn for audit logging.
type SpawnResult struct {
	SessionID    string
	SyncedFiles  int
	FailedSkills []string
	InspectPath  string
}

// SpawnOptions contains optional parameters for SpawnSession.
type SpawnOptions struct {
	Prompt        string // If set, run non-interactively with this prompt
	MaxIterations int    // CLI override for max tool iterations; 0 means use default
	Inspect       bool
}

type ResumeOptions struct {
	Inspect bool
}

func SpawnSession(cfg *agent.AgentConfig, opts ...SpawnOptions) (*SpawnResult, error) {
	var spawnOpts SpawnOptions
	if len(opts) > 0 {
		spawnOpts = opts[0]
	}
	sessionCfg := runtime.ResolveSessionConfig(cfg, runtime.ResolveOptions{
		MaxIterationsOverride: spawnOpts.MaxIterations,
	})
	if err := runtime.ValidateSessionConfig(sessionCfg); err != nil {
		return nil, err
	}
	provider, err := runtime.Get(sessionCfg.Runtime)
	if err != nil {
		return nil, err
	}
	workspace, _ := filepath.Abs(".")

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
	spawnOK := false
	defer func() {
		if !spawnOK {
			os.RemoveAll(workDir)
		}
	}()

	srcDir, err := filepath.Abs(agent.Dir(cfg.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent directory: %w", err)
	}

	if err := fileutil.CopyDir(srcDir, workDir); err != nil {
		return nil, fmt.Errorf("failed to copy agent template: %w", err)
	}

	if err := runtime.SaveSessionConfigInWorkspace(workspace, sessionID, sessionCfg); err != nil {
		return nil, fmt.Errorf("failed to write session config: %w", err)
	}

	if err := provider.PrepareSession(workDir, srcDir, sessionCfg, sessionID); err != nil {
		return nil, fmt.Errorf("failed to prepare runtime session: %w", err)
	}

	var failedSkills []string
	if len(sessionCfg.Skills) > 0 {
		failedSkills = resolveSkills(provider.SkillsDir(workDir), sessionCfg.Skills)
	}

	// Write resolved permission manifest for integration gateway enforcement
	if err := writePermissionManifest(filepath.Join(workspace, ".toc", "sessions", sessionID), sessionID, sessionCfg); err != nil {
		return nil, fmt.Errorf("failed to write permission manifest: %w", err)
	}

	if err := session.Add(session.Session{
		ID:            sessionID,
		Agent:         cfg.Name,
		Runtime:       sessionCfg.Runtime,
		MetadataDir:   filepath.Join(workspace, ".toc", "sessions", sessionID),
		CreatedAt:     time.Now(),
		WorkspacePath: workDir,
		Status:        session.StatusActive,
	}); err != nil {
		return nil, fmt.Errorf("failed to track session: %w", err)
	}
	spawnOK = true // session tracked — keep workspace on disk

	fmt.Println()
	ui.Info("Agent: %s", ui.Bold(sessionCfg.Agent))
	ui.Info("Model: %s", ui.Bold(sessionCfg.Model))
	ui.Info("Session: %s", ui.Cyan(sessionID))
	ui.Info("Workspace: %s", ui.Dim(workDir))
	if spawnOpts.Inspect {
		ui.Info("Inspect: %s", ui.Dim(runtime.InspectCapturePathInWorkspace(workspace, sessionID)))
	}
	if len(sessionCfg.Skills) > 0 {
		resolved := len(sessionCfg.Skills) - len(failedSkills)
		ui.Info("Skills: %s", ui.Dim(fmt.Sprintf("%d/%d resolved", resolved, len(sessionCfg.Skills))))
	}
	if len(sessionCfg.Context) > 0 {
		ui.Info("Context sync: %s", ui.Dim(fmt.Sprintf("%d pattern(s)", len(sessionCfg.Context))))
	}
	if sessionCfg.OnEnd != "" {
		ui.Info("On end: %s", ui.Dim("session end hook enabled"))
	}
	ui.Info("Integrations: %s", ui.Dim(integrationsSummary(sessionCfg)))
	if permissionsConfigured(sessionCfg) {
		ui.Info("Permissions: %s", ui.Dim("enforced via hooks"))
	}
	fmt.Println()

	if err := provider.LaunchInteractive(runtime.LaunchOptions{
		Dir: workDir, Model: sessionCfg.Model, SessionID: sessionID,
		AgentName: sessionCfg.Agent, Workspace: workspace,
		Prompt: spawnOpts.Prompt, Inspect: spawnOpts.Inspect,
	}); err != nil {
		_ = session.UpdateStatus(sessionID, session.StatusCompletedError)
		return nil, fmt.Errorf("failed to launch runtime session: %w", err)
	}
	_ = session.UpdateStatus(sessionID, session.StatusCompleted)

	// Post-session sync: copy matching files back to agent template
	var syncedFiles int
	if len(sessionCfg.Context) > 0 {
		syncedFiles = runPostSessionSync(provider, workDir, srcDir, sessionCfg.Context)
	}

	printFailedSkills(failedSkills)
	printResumeCommand(sessionCfg.Agent, sessionID)

	return &SpawnResult{
		SessionID:    sessionID,
		SyncedFiles:  syncedFiles,
		FailedSkills: failedSkills,
		InspectPath:  runtime.InspectCapturePathInWorkspace(workspace, sessionID),
	}, nil
}

func ResumeSession(s *session.Session, opts ...ResumeOptions) (*SpawnResult, error) {
	var resumeOpts ResumeOptions
	if len(opts) > 0 {
		resumeOpts = opts[0]
	}
	if _, err := os.Stat(s.WorkspacePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("session workspace no longer exists: %s", s.WorkspacePath)
	}

	workspace, _ := filepath.Abs(".")
	sessionCfg, err := loadOrResolveSessionConfig(workspace, s.ID, func() (*agent.AgentConfig, error) {
		return agent.Load(s.Agent)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load session config: %w", err)
	}
	if err := runtime.ValidateSessionConfig(sessionCfg); err != nil {
		return nil, fmt.Errorf("invalid session config: %w", err)
	}
	provider, err := runtime.Get(sessionCfg.Runtime)
	if err != nil {
		return nil, err
	}

	srcDir, err := filepath.Abs(agent.Dir(s.Agent))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent directory: %w", err)
	}

	// Re-resolve skills in case they were cleaned up or updated
	var failedSkills []string
	if len(sessionCfg.Skills) > 0 {
		failedSkills = resolveSkills(provider.SkillsDir(s.WorkspacePath), sessionCfg.Skills)
	}

	if err := provider.PrepareSession(s.WorkspacePath, srcDir, sessionCfg, s.ID); err != nil {
		return nil, fmt.Errorf("failed to prepare runtime session: %w", err)
	}

	fmt.Println()
	ui.Info("Resuming agent: %s", ui.Bold(s.Agent))
	ui.Info("Session: %s", ui.Cyan(s.ID))
	ui.Info("Workspace: %s", ui.Dim(s.WorkspacePath))
	if resumeOpts.Inspect {
		ui.Info("Inspect: %s", ui.Dim(runtime.InspectCapturePathInWorkspace(workspace, s.ID)))
	}
	if len(sessionCfg.Skills) > 0 {
		resolved := len(sessionCfg.Skills) - len(failedSkills)
		ui.Info("Skills: %s", ui.Dim(fmt.Sprintf("%d/%d resolved", resolved, len(sessionCfg.Skills))))
	}
	ui.Info("Integrations: %s", ui.Dim(integrationsSummary(sessionCfg)))
	fmt.Println()

	_ = session.UpdateStatus(s.ID, session.StatusActive)
	if err := provider.LaunchInteractive(runtime.LaunchOptions{
		Dir: s.WorkspacePath, Model: sessionCfg.Model, SessionID: s.ID,
		AgentName: s.Agent, Workspace: workspace, Resume: true, Inspect: resumeOpts.Inspect,
	}); err != nil {
		_ = session.UpdateStatus(s.ID, session.StatusCompletedError)
		return nil, fmt.Errorf("failed to resume runtime session: %w", err)
	}
	_ = session.UpdateStatus(s.ID, session.StatusCompleted)

	var syncedFiles int
	if len(sessionCfg.Context) > 0 {
		syncedFiles = runPostSessionSync(provider, s.WorkspacePath, srcDir, sessionCfg.Context)
	}

	printFailedSkills(failedSkills)
	printResumeCommand(s.Agent, s.ID)

	return &SpawnResult{
		SessionID:    s.ID,
		SyncedFiles:  syncedFiles,
		FailedSkills: failedSkills,
		InspectPath:  runtime.InspectCapturePathInWorkspace(workspace, s.ID),
	}, nil
}

// SubSpawnOpts contains options for spawning a sub-agent session.
type SubSpawnOpts struct {
	ParentSessionID string
	Prompt          string
	WorkspaceDir    string // absolute path to the toc workspace
	MaxIterations   int    // CLI override for max tool iterations; 0 means use default
	Inspect         bool
}

// SpawnSubSession spawns a non-interactive sub-agent session in the background.
// Output is captured to toc-output.txt in the session workspace.
func SpawnSubSession(cfg *agent.AgentConfig, opts SubSpawnOpts) (*SpawnResult, error) {
	sessionCfg := runtime.ResolveSessionConfig(cfg, runtime.ResolveOptions{
		MaxIterationsOverride: opts.MaxIterations,
	})
	restrictNativeSubAgentTools(sessionCfg)
	if err := runtime.ValidateSessionConfig(sessionCfg); err != nil {
		return nil, err
	}
	provider, err := runtime.Get(sessionCfg.Runtime)
	if err != nil {
		return nil, err
	}

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
	spawnOK := false
	defer func() {
		if !spawnOK {
			os.RemoveAll(workDir)
		}
	}()

	// Use workspace-relative agent dir
	agentDir := filepath.Join(opts.WorkspaceDir, ".toc", "agents", cfg.Name)
	if err := fileutil.CopyDir(agentDir, workDir); err != nil {
		return nil, fmt.Errorf("failed to copy agent template: %w", err)
	}

	if err := runtime.SaveSessionConfigInWorkspace(opts.WorkspaceDir, sessionID, sessionCfg); err != nil {
		return nil, fmt.Errorf("failed to write session config: %w", err)
	}

	if err := provider.PrepareSession(workDir, agentDir, sessionCfg, sessionID); err != nil {
		return nil, fmt.Errorf("failed to prepare runtime session: %w", err)
	}

	var failedSkills []string
	if len(sessionCfg.Skills) > 0 {
		failedSkills = resolveSkills(provider.SkillsDir(workDir), sessionCfg.Skills)
	}

	// Write resolved permission manifest for sub-agent
	if err := writePermissionManifestInWorkspace(opts.WorkspaceDir, sessionID, sessionCfg); err != nil {
		return nil, fmt.Errorf("failed to write permission manifest: %w", err)
	}

	if err := session.AddInWorkspace(opts.WorkspaceDir, session.Session{
		ID:              sessionID,
		Name:            naming.FromPrompt(opts.Prompt),
		Agent:           sessionCfg.Agent,
		Runtime:         sessionCfg.Runtime,
		MetadataDir:     filepath.Join(opts.WorkspaceDir, ".toc", "sessions", sessionID),
		CreatedAt:       time.Now(),
		WorkspacePath:   workDir,
		Status:          session.StatusActive,
		ParentSessionID: opts.ParentSessionID,
		Prompt:          opts.Prompt,
	}); err != nil {
		return nil, fmt.Errorf("failed to track session: %w", err)
	}
	spawnOK = true // session tracked — keep workspace on disk

	// Launch the runtime as a detached background process so it survives after toc exits.
	outputPath := filepath.Join(workDir, "toc-output.txt")
	if err := provider.LaunchDetached(runtime.DetachedOptions{
		Dir: workDir, Model: sessionCfg.Model, Prompt: opts.Prompt,
		Workspace: opts.WorkspaceDir, AgentName: sessionCfg.Agent,
		SessionID: sessionID, ParentSessionID: opts.ParentSessionID, OutputPath: outputPath,
		Inspect: opts.Inspect,
	}); err != nil {
		return nil, fmt.Errorf("failed to launch sub-agent: %w", err)
	}

	return &SpawnResult{
		SessionID:    sessionID,
		FailedSkills: failedSkills,
		InspectPath:  runtime.InspectCapturePathInWorkspace(opts.WorkspaceDir, sessionID),
	}, nil
}

// SubResumeOpts contains options for resuming a sub-agent session.
type SubResumeOpts struct {
	ParentSessionID string
	Prompt          string // optional additional context for the resumed session
	WorkspaceDir    string
	Inspect         bool
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
	sessionCfg, err := loadOrResolveSessionConfig(opts.WorkspaceDir, s.ID, func() (*agent.AgentConfig, error) {
		return agent.LoadFrom(agentDir)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load session config: %w", err)
	}
	restrictNativeSubAgentTools(sessionCfg)
	if err := runtime.ValidateSessionConfig(sessionCfg); err != nil {
		return nil, fmt.Errorf("invalid session config: %w", err)
	}
	provider, err := runtime.Get(sessionCfg.Runtime)
	if err != nil {
		return nil, err
	}

	if err := provider.PrepareSession(s.WorkspacePath, agentDir, sessionCfg, s.ID); err != nil {
		return nil, fmt.Errorf("failed to prepare runtime session: %w", err)
	}

	var failedSkills []string
	if len(sessionCfg.Skills) > 0 {
		failedSkills = resolveSkills(provider.SkillsDir(s.WorkspacePath), sessionCfg.Skills)
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

	// Launch the runtime detached with resume support.
	outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
	if err := provider.LaunchDetached(runtime.DetachedOptions{
		Dir: s.WorkspacePath, Model: sessionCfg.Model, Prompt: resumePrompt,
		Workspace: opts.WorkspaceDir, AgentName: s.Agent, SessionID: s.ID,
		ParentSessionID: s.ParentSessionID, OutputPath: outputPath, Resume: true, Inspect: opts.Inspect,
	}); err != nil {
		return nil, fmt.Errorf("failed to launch resumed sub-agent: %w", err)
	}

	return &SpawnResult{
		SessionID:    s.ID,
		FailedSkills: failedSkills,
		InspectPath:  runtime.InspectCapturePathInWorkspace(opts.WorkspaceDir, s.ID),
	}, nil
}

func runPostSessionSync(provider runtime.Provider, workDir, agentDir string, patterns []string) int {
	synced, err := provider.PostSessionSync(workDir, agentDir, patterns)
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

func restrictNativeSubAgentTools(cfg *runtime.SessionConfig) {
	if cfg == nil || cfg.Runtime != runtimeinfo.NativeRuntime {
		return
	}
	// An empty EnabledTools list currently means "no tools" for toc-native,
	// so there's nothing to filter. If that invariant ever changes to mean
	// "all tools", this helper must be revisited.
	if len(cfg.RuntimeConfig.EnabledTools) == 0 {
		return
	}

	disabled := map[string]bool{
		"TodoWrite": true,
		"SubAgent":  true,
	}
	filtered := make([]string, 0, len(cfg.RuntimeConfig.EnabledTools))
	for _, tool := range cfg.RuntimeConfig.EnabledTools {
		if disabled[tool] {
			continue
		}
		filtered = append(filtered, tool)
	}
	cfg.RuntimeConfig.EnabledTools = filtered
}

func printResumeCommand(agentName, sessionID string) {
	fmt.Println()
	fmt.Printf("  %s\n", ui.Dim("───────────────────────────────────────"))
	ui.Info("Session ended. Resume with:")
	fmt.Println()
	ui.Command(fmt.Sprintf("toc agent spawn %s --resume %s", agentName, sessionID))
	fmt.Println()
}

// resolveSkills resolves all skill references and copies them into the runtime's
// skills directory. Returns a list of skill entries that failed to resolve.
func resolveSkills(skillsTarget string, skills []string) []string {
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

// writePermissionManifest writes the resolved session permissions to
// .toc/sessions/<id>/permissions.json. This manifest is immutable after spawn —
// the running session keeps its original permissions even if oc-agent.yaml changes.
func writePermissionManifest(metadataDir, sessionID string, cfg *runtime.SessionConfig) error {
	perms := cfg.Permissions

	manifest := integration.PermissionManifest{
		SessionID:    sessionID,
		Agent:        cfg.Agent,
		Filesystem:   perms.Filesystem,
		Integrations: perms.Integrations,
		SubAgents:    perms.SubAgents,
	}

	if err := os.MkdirAll(metadataDir, 0700); err != nil {
		return fmt.Errorf("failed to create session permissions directory: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(metadataDir, "permissions.json"), data, 0600)
}

// writePermissionManifestInWorkspace writes the permission manifest using an
// explicit workspace path. Used for sub-agent spawning.
func writePermissionManifestInWorkspace(workspace, sessionID string, cfg *runtime.SessionConfig) error {
	return writePermissionManifest(filepath.Join(workspace, ".toc", "sessions", sessionID), sessionID, cfg)
}

func integrationsSummary(cfg *runtime.SessionConfig) string {
	if cfg == nil || len(cfg.Permissions.Integrations) == 0 {
		return "none"
	}
	names := make([]string, 0, len(cfg.Permissions.Integrations))
	for name := range cfg.Permissions.Integrations {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, len(names))
	for i, name := range names {
		n := len(cfg.Permissions.Integrations[name])
		parts[i] = fmt.Sprintf("%s (%d action(s))", name, n)
	}
	return strings.Join(parts, ", ")
}

func permissionsConfigured(cfg *runtime.SessionConfig) bool {
	if cfg == nil {
		return false
	}
	perms := cfg.Permissions
	return len(perms.Integrations) > 0 ||
		len(perms.SubAgents) > 0 ||
		perms.Filesystem.Read != agent.PermOn ||
		perms.Filesystem.Write != agent.PermOn ||
		perms.Filesystem.Execute != agent.PermOn
}

func loadOrResolveSessionConfig(workspace, sessionID string, fallback func() (*agent.AgentConfig, error)) (*runtime.SessionConfig, error) {
	cfg, err := runtime.LoadSessionConfigInWorkspace(workspace, sessionID)
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	agentCfg, err := fallback()
	if err != nil {
		return nil, err
	}
	cfg = runtime.ResolveSessionConfig(agentCfg)
	if saveErr := runtime.SaveSessionConfigInWorkspace(workspace, sessionID, cfg); saveErr != nil {
		return nil, saveErr
	}
	return cfg, nil
}
