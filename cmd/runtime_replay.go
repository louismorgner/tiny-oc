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
	Long:  "Parse Claude Code session logs and show a structured timeline of agent behavior including reasoning, tool calls, and skill invocations.",
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
	fmt.Printf("  %s: %s — agent: %s — %s — %s\n",
		ui.Bold("Session"),
		ui.Cyan(shortID(r.SessionID)),
		ui.Cyan(r.Agent),
		ui.Dim(r.FormatDuration()),
		ui.Dim(tokenStr),
	)
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
	fmt.Printf("  %s: %d | %s: %d | %s: %s | %s: %d\n",
		ui.Bold("Files changed"), len(r.FilesChanged),
		ui.Bold("Tools"), r.ToolCount,
		ui.Bold("Tokens"), tokenStr,
		ui.Bold("Errors"), r.ErrorCount,
	)
	fmt.Println()
	return nil
}

// printOpts controls how steps are rendered.
type printOpts struct {
	HideText    bool // compact mode: don't show text/assistant steps
	FullContent bool // verbose mode: show full content without truncation
}

func printStep(step replay.Step, opts printOpts) {
	switch step.Type {
	case "thinking":
		if opts.FullContent {
			printMultiLine("[think]    ", step.Content, ui.Dim)
		} else {
			text := replay.TruncateThinking(step.Content, 100)
			fmt.Printf("  %s %s\n", ui.Dim("[think]    "), ui.Dim(text))
		}
	case "text":
		if !opts.HideText {
			if opts.FullContent {
				printMultiLine("[assistant]", step.Content, nil)
			} else {
				text := replay.TruncateThinking(step.Content, 120)
				fmt.Printf("  %s %s\n", ui.Cyan("[assistant]"), text)
			}
		}
	case "tool":
		printToolStep(step)
	case "skill":
		fmt.Printf("  %s %s\n", ui.Yellow("[skill]    "), ui.Cyan(step.Skill))
	case "error":
		text := step.Content
		if len(text) > 100 {
			text = text[:97] + "..."
		}
		fmt.Printf("  %s %s\n", ui.Red("[error]    "), text)
	}
}

// printMultiLine prints a labeled block with wrapped/indented content.
func printMultiLine(label, content string, colorFn func(a ...interface{}) string) {
	lines := strings.Split(content, "\n")
	indent := strings.Repeat(" ", 2+len(label)+1) // "  " + label + " "
	for i, line := range lines {
		if colorFn != nil {
			line = colorFn(line)
		}
		if i == 0 {
			if colorFn != nil {
				fmt.Printf("  %s %s\n", ui.Dim(label), line)
			} else {
				fmt.Printf("  %s %s\n", ui.Cyan(label), line)
			}
		} else {
			fmt.Printf("%s%s\n", indent, line)
		}
	}
}

func printToolStep(step replay.Step) {
	switch step.Tool {
	case "Read":
		fmt.Printf("  %s %s\n", ui.Dim("[read]     "), shortPath(step.Path))
	case "Edit":
		detail := ""
		if step.Added > 0 || step.Removed > 0 {
			detail = fmt.Sprintf(" +%d -%d lines", step.Added, step.Removed)
		}
		fmt.Printf("  %s %s%s\n", ui.Dim("[edit]     "), shortPath(step.Path), ui.Dim(detail))
	case "Write":
		detail := ""
		if step.Lines > 0 {
			detail = fmt.Sprintf(" %d lines", step.Lines)
		}
		fmt.Printf("  %s %s%s\n", ui.Dim("[write]    "), shortPath(step.Path), ui.Dim(detail))
	case "Bash":
		cmd := step.Command
		if len(cmd) > 80 {
			cmd = cmd[:77] + "..."
		}
		fmt.Printf("  %s %s\n", ui.Dim("[bash]     "), cmd)
	case "Glob":
		fmt.Printf("  %s %s\n", ui.Dim("[glob]     "), step.Content)
	case "Grep":
		fmt.Printf("  %s %s\n", ui.Dim("[grep]     "), step.Content)
	default:
		detail := step.Tool
		if step.Path != "" {
			detail += " " + shortPath(step.Path)
		}
		fmt.Printf("  %s %s\n", ui.Dim("[tool]     "), detail)
	}
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
