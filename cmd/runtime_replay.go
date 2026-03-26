package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/replay"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeReplayCmd.Flags().Bool("json", false, "Output structured JSON")
	runtimeReplayCmd.Flags().Bool("thinking-only", false, "Show only thinking/reasoning steps")
	runtimeReplayCmd.Flags().Bool("actions-only", false, "Show only tool calls and skills")
	runtimeReplayCmd.Flags().Bool("compact", false, "One line per action, no thinking")
	runtimeReplayCmd.Flags().Bool("full", false, "Show all steps including tool calls (default hides them)")
	runtimeCmd.AddCommand(runtimeReplayCmd)
}

var runtimeReplayCmd = &cobra.Command{
	Use:   "replay <session-id>",
	Short: "Replay a session timeline — thinking, tools, and skills",
	Long:  "Parse runtime session logs and show a structured timeline of agent behavior including reasoning, tool calls, and skill invocations.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]

		var s *session.Session

		// Try workspace-scoped lookup first if inside a toc session
		if ctx, err := runtime.FromEnv(); err == nil {
			s, _ = session.FindByIDInWorkspace(ctx.Workspace, sessionID)
			if s == nil {
				s, _ = session.FindByIDPrefixInWorkspace(ctx.Workspace, sessionID)
			}
		}

		// Fall back to global sessions
		if s == nil {
			var err error
			s, err = session.FindByID(sessionID)
			if err != nil {
				s, err = session.FindByIDPrefix(sessionID)
				if err != nil {
					return fmt.Errorf("session '%s' not found", sessionID)
				}
			}
		}

		r, err := replay.ForSession(s)
		if err != nil {
			return err
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			return printReplayJSON(r)
		}

		thinkingOnly, _ := cmd.Flags().GetBool("thinking-only")
		actionsOnly, _ := cmd.Flags().GetBool("actions-only")
		compact, _ := cmd.Flags().GetBool("compact")
		full, _ := cmd.Flags().GetBool("full")

		return printReplayHuman(r, thinkingOnly, actionsOnly, compact, full)
	},
}

func printReplayJSON(r *replay.Replay) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printReplayHuman(r *replay.Replay, thinkingOnly, actionsOnly, compact, full bool) error {
	tokenStr := r.Tokens.FormatTotal()
	if tokenStr == "" {
		tokenStr = "0 tokens"
	}

	fmt.Println()
	fmt.Printf("  %s  %s\n", ui.BoldCyan(r.Agent), ui.Dim(shortID(r.SessionID)))
	details := ""
	if r.Runtime != "" {
		details += r.Runtime
	}
	if r.Model != "" {
		details += "/" + r.Model
	}
	if r.Status != "" {
		if details != "" {
			details += "  "
		}
		details += r.Status
	}
	details += "  " + r.FormatDuration() + "  " + tokenStr
	fmt.Printf("  %s\n", ui.Dim(details))
	fmt.Println(ui.TurnSeparator())
	fmt.Println()

	for _, step := range r.Steps {
		if thinkingOnly && step.Type != "thinking" {
			continue
		}
		if actionsOnly && step.Type != "tool" && step.Type != "skill" {
			continue
		}
		if compact && step.Type == "thinking" {
			continue
		}
		// Default: hide tool calls (implementation noise). Show with --full or --actions-only.
		if step.Type == "tool" && !full && !actionsOnly {
			continue
		}

		printStep(step, printOpts{HideText: compact})
	}

	fmt.Println()
	fmt.Println(ui.TurnSeparator())
	fmt.Println()
	fmt.Printf("  %s %d   %s %d   %s %s   %s %d\n",
		ui.Bold("files:"), len(r.FilesChanged),
		ui.Bold("tools:"), r.ToolCount,
		ui.Bold("tokens:"), tokenStr,
		ui.Bold("errors:"), r.ErrorCount,
	)
	if r.ResumeCount > 0 {
		fmt.Printf("  %s %d\n", ui.Bold("resumes:"), r.ResumeCount)
	}
	if r.RecoveryCount > 0 {
		fmt.Printf("  %s %d\n", ui.Bold("recoveries:"), r.RecoveryCount)
	}
	if r.CompactionCount > 0 {
		fmt.Printf("  %s %d\n", ui.Bold("compactions:"), r.CompactionCount)
	}
	if r.LastError != "" && r.Status != session.StatusCompletedOK && r.Status != "completed" {
		fmt.Printf("  %s %s\n", ui.Bold("last error:"), ui.Dim(r.LastError))
	}
	if r.LastRecovery != "" {
		fmt.Printf("  %s %s\n", ui.Bold("last recovery:"), ui.Dim(r.LastRecovery))
	}
	fmt.Println()
	return nil
}

// printOpts controls how steps are rendered.
type printOpts struct {
	HideText    bool // compact mode: don't show text/assistant steps
	FullContent bool // verbose mode: show full content without truncation
}

func printStep(step runtime.Step, opts printOpts) {
	meta := ui.StepMeta{
		ToolName:   step.Tool,
		Path:       step.Path,
		Command:    step.Command,
		Skill:      step.Skill,
		Added:      step.Added,
		Removed:    step.Removed,
		Lines:      step.Lines,
		ExitCode:   step.ExitCode,
		TimedOut:   step.TimedOut,
		DurationMS: step.DurationMS,
		Full:       opts.FullContent,
	}

	if opts.HideText && step.Type == "text" {
		return
	}

	fmt.Print(ui.FormatStepRich(step.Type, step.Content, meta))
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func shortPath(p string) string {
	// Show last 2-3 path components for context, e.g. "internal/agent/agent.go"
	parts := strings.Split(filepath.ToSlash(p), "/")
	if len(parts) <= 3 {
		return p
	}
	return strings.Join(parts[len(parts)-3:], "/")
}
