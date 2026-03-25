package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
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

		if len(args) > 0 {
			return showSubAgentStatus(ctx, args[0])
		}

		return listSubAgentStatuses(ctx)
	},
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
	var badge string
	switch status {
	case "active":
		badge = ui.Green("● active")
	case "completed":
		badge = ui.Green("● completed")
	case "stale":
		badge = ui.Yellow("◌ stale")
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Bold("Agent:"), ui.Cyan(s.Agent))
	fmt.Printf("  %s %s\n", ui.Bold("Status:"), badge)
	fmt.Printf("  %s %s\n", ui.Bold("Session:"), ui.Dim(s.ID))
	if s.Prompt != "" {
		prompt := s.Prompt
		if len(prompt) > 80 {
			prompt = prompt[:77] + "..."
		}
		fmt.Printf("  %s %s\n", ui.Bold("Prompt:"), ui.Dim(prompt))
	}
	fmt.Println()

	if status == "completed" {
		ui.Info("Read output: %s", ui.Bold(fmt.Sprintf("toc runtime output %s", sessionID)))
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
	fmt.Printf("  %-10s %-16s %-10s %s\n", ui.Bold("STATUS"), ui.Bold("AGENT"), ui.Bold("SESSION"), ui.Bold("PROMPT"))
	fmt.Printf("  %-10s %-16s %-10s %s\n", ui.Dim("──────────"), ui.Dim("────────────────"), ui.Dim("──────────"), ui.Dim("────────────────────────────"))

	for _, s := range children {
		status := s.ResolvedStatus()
		var badge string
		switch status {
		case "active":
			badge = ui.Green("● active")
		case "completed":
			badge = ui.Green("● completed")
		case "stale":
			badge = ui.Yellow("◌ stale")
		}

		prompt := s.Prompt
		if len(prompt) > 40 {
			prompt = prompt[:37] + "..."
		}

		fmt.Printf("  %-10s %-16s %-10s %s\n", badge, ui.Cyan(s.Agent), ui.Dim(s.ID[:8]), ui.Dim(prompt))
	}

	fmt.Println()
	return nil
}
