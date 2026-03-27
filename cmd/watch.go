package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
)

func init() {
	watchCmd.Flags().Bool("json", false, "Output each step as newline-delimited JSON")
	watchCmd.Flags().BoolP("verbose", "v", false, "Show full assistant messages and thinking blocks")
	rootCmd.AddCommand(watchCmd)
}

var watchCmd = &cobra.Command{
	Use:   "watch <session-id>",
	Short: "Live-tail an agent session",
	Long:  "Stream formatted activity from a running agent session. Shows tool calls, skills, and text output in real time.",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 || !config.Exists() {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		sf, err := session.Load()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var completions []string
		for _, s := range sf.Sessions {
			status := s.ResolvedStatus()
			completions = append(completions, fmt.Sprintf("%s\t%s (%s)", s.ID, s.Agent, status))
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		sessionID := args[0]

		s, err := session.FindByID(sessionID)
		if err != nil {
			s, err = session.FindByIDPrefix(sessionID)
			if err != nil {
				return fmt.Errorf("session '%s' not found", sessionID)
			}
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		verbose, _ := cmd.Flags().GetBool("verbose")

		return runWatch(s, jsonFlag, verbose)
	},
}
