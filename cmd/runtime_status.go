package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeStatusCmd.Flags().Bool("json", false, "Output structured JSON")
	runtimeCmd.AddCommand(runtimeStatusCmd)
}

var runtimeStatusCmd = &cobra.Command{
	Use:   "status [session-id]",
	Short: "Check status of sub-agent sessions",
	Long:  "Without arguments, shows all sub-agents spawned by this session. With a session ID, shows details for that specific sub-agent.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")

		if len(args) > 0 {
			if jsonFlag {
				return showSubAgentStatusJSON(ctx, args[0])
			}
			return showSubAgentStatus(ctx, args[0])
		}

		if jsonFlag {
			return listSubAgentStatusesJSON(ctx)
		}
		return listSubAgentStatuses(ctx)
	},
}

type statusJSON struct {
	ID           string `json:"id"`
	Agent        string `json:"agent"`
	Status       string `json:"status"`
	Prompt       string `json:"prompt,omitempty"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	Model        string `json:"model,omitempty"`
	Runtime      string `json:"runtime,omitempty"`
	RuntimeState string `json:"runtime_state,omitempty"`
	ResumeCount  int    `json:"resume_count,omitempty"`
	Recoveries   int    `json:"recovery_count,omitempty"`
	Compactions  int    `json:"compactions,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	LastRecovery string `json:"last_recovery,omitempty"`
	TokenTotal   int64  `json:"token_total,omitempty"`
}

func statusJSONForSession(s *session.Session) statusJSON {
	sj := statusJSON{
		ID:      s.ID,
		Agent:   s.Agent,
		Status:  s.ResolvedStatus(),
		Prompt:  s.Prompt,
		Runtime: s.RuntimeName(),
	}
	if exitCode, err := s.ReadExitCode(); err == nil {
		sj.ExitCode = &exitCode
	}
	if summary, err := loadRuntimeStateSummary(s); err == nil && summary != nil {
		sj.Model = summary.Model
		sj.RuntimeState = summary.Status
		sj.ResumeCount = summary.ResumeCount
		sj.Recoveries = summary.RecoveryCount
		sj.Compactions = summary.CompactionCount
		sj.LastError = summary.LastError
		sj.LastRecovery = summary.LastRecovery
		sj.TokenTotal = summary.Tokens.Total()
	}
	return sj
}

func showSubAgentStatusJSON(ctx *runtime.Context, sessionID string) error {
	s, err := session.FindByIDInWorkspace(ctx.Workspace, sessionID)
	if err != nil {
		return err
	}
	if s.ParentSessionID != ctx.SessionID {
		return fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID)
	}
	data, err := json.Marshal(statusJSONForSession(s))
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func listSubAgentStatusesJSON(ctx *runtime.Context) error {
	children, err := session.ListByParentInWorkspace(ctx.Workspace, ctx.SessionID)
	if err != nil {
		return err
	}
	var result []statusJSON
	for _, s := range children {
		result = append(result, statusJSONForSession(&s))
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func statusBadge(status string) string {
	switch status {
	case "active":
		return ui.Green("● active")
	case "completed", session.StatusCompletedOK:
		return ui.Green("● completed")
	case session.StatusCompletedError:
		return ui.Red("✗ failed")
	case session.StatusZombie:
		return ui.Red("⚠ zombie")
	case session.StatusCancelled:
		return ui.Yellow("◼ cancelled")
	case "stale":
		return ui.Yellow("◌ stale")
	default:
		return ui.Dim(status)
	}
}

func showSubAgentStatus(ctx *runtime.Context, sessionID string) error {
	s, err := session.FindByIDInWorkspace(ctx.Workspace, sessionID)
	if err != nil {
		return err
	}

	if s.ParentSessionID != ctx.SessionID {
		return fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID)
	}

	status := s.ResolvedStatus()
	badge := statusBadge(status)

	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Bold("Agent:"), ui.Cyan(s.Agent))
	fmt.Printf("  %s %s\n", ui.Bold("Status:"), badge)
	fmt.Printf("  %s %s\n", ui.Bold("Session:"), ui.Dim(s.ID))
	fmt.Printf("  %s %s\n", ui.Bold("Runtime:"), ui.Dim(s.RuntimeName()))
	if exitCode, err := s.ReadExitCode(); err == nil {
		fmt.Printf("  %s %d\n", ui.Bold("Exit code:"), exitCode)
	}
	if summary, err := loadRuntimeStateSummary(s); err == nil && summary != nil {
		if summary.Model != "" {
			fmt.Printf("  %s %s\n", ui.Bold("Model:"), ui.Dim(summary.Model))
		}
		if summary.Status != "" {
			fmt.Printf("  %s %s\n", ui.Bold("State:"), ui.Dim(summary.Status))
		}
		if summary.ResumeCount > 0 {
			fmt.Printf("  %s %d\n", ui.Bold("Resumes:"), summary.ResumeCount)
		}
		if summary.RecoveryCount > 0 {
			fmt.Printf("  %s %d\n", ui.Bold("Recoveries:"), summary.RecoveryCount)
		}
		if summary.CompactionCount > 0 {
			fmt.Printf("  %s %d\n", ui.Bold("Compactions:"), summary.CompactionCount)
		}
		if total := summary.Tokens.FormatTotal(); total != "" {
			fmt.Printf("  %s %s\n", ui.Bold("Tokens:"), ui.Dim(total))
		}
		if summary.LastError != "" {
			fmt.Printf("  %s %s\n", ui.Bold("Last error:"), ui.Dim(summary.LastError))
		}
		if summary.LastRecovery != "" {
			fmt.Printf("  %s %s\n", ui.Bold("Last recovery:"), ui.Dim(summary.LastRecovery))
		}
	}
	if s.Prompt != "" {
		prompt := s.Prompt
		if len(prompt) > 80 {
			prompt = prompt[:77] + "..."
		}
		fmt.Printf("  %s %s\n", ui.Bold("Prompt:"), ui.Dim(prompt))
	}
	fmt.Println()

	switch status {
	case "completed", session.StatusCompletedOK:
		ui.Info("Read output: %s", ui.Bold(fmt.Sprintf("toc runtime output %s", sessionID)))
		fmt.Println()
	case session.StatusCompletedError, session.StatusZombie:
		ui.Info("Read output:  %s", ui.Bold(fmt.Sprintf("toc runtime output %s", sessionID)))
		ui.Info("Resume:       %s", ui.Bold(fmt.Sprintf("toc runtime spawn %s --resume %s", s.Agent, sessionID)))
		fmt.Println()
	case session.StatusCancelled:
		ui.Info("Resume: %s", ui.Bold(fmt.Sprintf("toc runtime spawn %s --resume %s", s.Agent, sessionID)))
		fmt.Println()
	}

	return nil
}

func listSubAgentStatuses(ctx *runtime.Context) error {
	children, err := session.ListByParentInWorkspace(ctx.Workspace, ctx.SessionID)
	if err != nil {
		return err
	}

	if len(children) == 0 {
		ui.Info("No sub-agents spawned by this session.")
		return nil
	}

	fmt.Println()
	fmt.Printf("  %-10s %-16s %-10s %-10s %s\n", ui.Bold("STATUS"), ui.Bold("AGENT"), ui.Bold("SESSION"), ui.Bold("TOKENS"), ui.Bold("PROMPT"))
	fmt.Printf("  %-10s %-16s %-10s %-10s %s\n", ui.Dim("──────────"), ui.Dim("────────────────"), ui.Dim("──────────"), ui.Dim("──────────"), ui.Dim("────────────────────────────"))

	for _, s := range children {
		badge := statusBadge(s.ResolvedStatus())
		tokenText := ""
		if summary, err := loadRuntimeStateSummary(&s); err == nil && summary != nil {
			tokenText = summary.Tokens.FormatTotal()
			if tokenText == "" && summary.ResumeCount > 0 {
				tokenText = fmt.Sprintf("resume:%d", summary.ResumeCount)
			}
			if tokenText == "" && summary.RecoveryCount > 0 {
				tokenText = fmt.Sprintf("recover:%d", summary.RecoveryCount)
			}
		}
		if len(tokenText) > 10 {
			tokenText = tokenText[:10]
		}

		prompt := s.Prompt
		if len(prompt) > 40 {
			prompt = prompt[:37] + "..."
		}

		fmt.Printf("  %-10s %-16s %-10s %-10s %s\n", badge, ui.Cyan(s.Agent), ui.Dim(s.ID[:8]), ui.Dim(tokenText), ui.Dim(prompt))
	}

	fmt.Println()
	return nil
}
