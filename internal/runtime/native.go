package runtime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	tocsync "github.com/tiny-oc/toc/internal/sync"
)

const nativeCommandName = "__native-run"

type nativeProvider struct{}

func (nativeProvider) Name() string { return runtimeinfo.NativeRuntime }

func (nativeProvider) DefaultModel() string {
	return runtimeinfo.DefaultModel(runtimeinfo.NativeRuntime)
}

func (nativeProvider) ModelOptions() []ModelOption {
	options := runtimeinfo.ModelOptions(runtimeinfo.NativeRuntime)
	result := make([]ModelOption, 0, len(options))
	for _, opt := range options {
		result = append(result, ModelOption{
			ID:          opt.ID,
			Label:       opt.Label,
			Description: opt.Description,
		})
	}
	return result
}

func (nativeProvider) ValidateModel(model string) error {
	return runtimeinfo.ValidateModel(runtimeinfo.NativeRuntime, model)
}

func (nativeProvider) PrepareSession(workDir, agentDir string, cfg *SessionConfig, sessionID string) error {
	nativeDir := filepath.Join(workDir, ".toc-native")
	if err := os.MkdirAll(filepath.Join(nativeDir, "skills"), 0755); err != nil {
		return err
	}

	content, err := ComposePrompt(workDir, cfg, sessionID)
	if err != nil {
		return err
	}
	if content == "" {
		return nil
	}

	return os.WriteFile(filepath.Join(nativeDir, "system-prompt.md"), []byte(content+"\n"), 0644)
}

func (nativeProvider) SkillsDir(workDir string) string {
	return filepath.Join(workDir, ".toc-native", "skills")
}

func (nativeProvider) PostSessionSync(workDir, agentDir string, patterns []string) ([]string, error) {
	return tocsync.SyncBackWithOptions(workDir, agentDir, patterns, tocsync.Options{
		SkipDirs: map[string]bool{".toc-native": true},
	})
}

func (nativeProvider) LaunchInteractive(opts LaunchOptions) error {
	exe, err := nativeExecutable()
	if err != nil {
		return err
	}

	args := []string{nativeCommandName, "--mode", "interactive", "--dir", opts.Dir, "--session-id", opts.SessionID, "--agent", opts.AgentName, "--workspace", opts.Workspace}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Resume {
		args = append(args, "--resume")
	}

	// When a prompt is provided, write it to a file and pass --prompt-file
	// so the runtime runs non-interactively in the foreground.
	if opts.Prompt != "" {
		promptPath := filepath.Join(opts.Dir, "toc-prompt.txt")
		if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0644); err != nil {
			return fmt.Errorf("failed to write prompt file: %w", err)
		}
		args = append(args, "--prompt-file", promptPath)
	}

	cmd := exec.Command(exe, args...)
	cmd.Dir = opts.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"TOC_WORKSPACE="+opts.Workspace,
		"TOC_AGENT="+opts.AgentName,
		"TOC_SESSION_ID="+opts.SessionID,
	)

	// Inject stored OpenRouter key if not already in the environment
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		if key := config.OpenRouterKey(); key != "" {
			cmd.Env = append(cmd.Env, "OPENROUTER_API_KEY="+key)
		}
	}

	// Ignore SIGINT in the parent while the child runs. The child process
	// handles SIGINT itself (graceful finalization). Without this, Go's
	// default handler kills the parent before post-session hooks can run.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	if err := cmd.Run(); err != nil {
		// Swallow ExitError (e.g. from Ctrl+C / SIGINT) so that the caller
		// can proceed with post-session hooks, matching claude.go behavior.
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return fmt.Errorf("failed to launch toc-native runtime: %w", err)
	}
	return nil
}

func (nativeProvider) LaunchDetached(opts DetachedOptions) error {
	promptPath := filepath.Join(opts.Dir, "toc-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0644); err != nil {
		return err
	}

	exe, err := nativeExecutable()
	if err != nil {
		return err
	}

	scriptPath := filepath.Join(opts.Dir, "toc-run.sh")
	if err := os.WriteFile(scriptPath, []byte(BuildNativeDetachedScript(exe, opts, promptPath)), 0755); err != nil {
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

func BuildNativeDetachedScript(executable string, opts DetachedOptions, promptPath string) string {
	tmpOutputPath := opts.OutputPath + ".tmp"
	pidPath := filepath.Join(opts.Dir, "toc-pid.txt")
	exitCodePath := filepath.Join(opts.Dir, "toc-exit-code.txt")

	command := fmt.Sprintf("%q %s --mode detached --dir %q --session-id %q --agent %q --workspace %q --prompt-file %q",
		executable, nativeCommandName, opts.Dir, opts.SessionID, opts.AgentName, opts.Workspace, promptPath)
	if opts.Model != "" {
		command += fmt.Sprintf(" --model %q", opts.Model)
	}
	if opts.Resume {
		command += " --resume"
	}

	// Inject stored OpenRouter key into detached script if not in current env
	openRouterExport := ""
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		if key := config.OpenRouterKey(); key != "" {
			openRouterExport = fmt.Sprintf("export OPENROUTER_API_KEY=%q\n", key)
		}
	}

	return fmt.Sprintf(`#!/bin/sh
echo $$ > %q
cd %q
export TOC_WORKSPACE=%q
export TOC_AGENT=%q
export TOC_SESSION_ID=%q
%s%s < /dev/null > %q 2>&1
TOC_EXIT=$?
echo $TOC_EXIT > %q
mv %q %q
`, pidPath, opts.Dir, opts.Workspace, opts.AgentName, opts.SessionID, openRouterExport, command, tmpOutputPath, exitCodePath, tmpOutputPath, opts.OutputPath)
}

func nativeExecutable() (string, error) {
	if path := os.Getenv("TOC_NATIVE_EXECUTABLE"); path != "" {
		return path, nil
	}
	return os.Executable()
}

func (nativeProvider) SessionLogPath(sess *session.Session) string {
	return EventLogPath(sess)
}

func (nativeProvider) ExpectedSessionLogPath(sess *session.Session) string {
	return EventLogPath(sess)
}

func (nativeProvider) ParseSessionLog(path string) (*ParsedLog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open session log: %w", err)
	}
	defer f.Close()

	result := &ParsedLog{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		result.Events = append(result.Events, event)
		result.Steps = append(result.Steps, event.Step)
		if !event.Timestamp.IsZero() {
			if result.FirstTS.IsZero() {
				result.FirstTS = event.Timestamp
			}
			result.LastTS = event.Timestamp
		}
	}
	return result, scanner.Err()
}

func (nativeProvider) ParseSessionLogLineEvents(line []byte) []Event {
	var event Event
	if err := json.Unmarshal(line, &event); err != nil {
		return nil
	}
	if event.Step.Type == "" {
		return nil
	}
	return []Event{event}
}
