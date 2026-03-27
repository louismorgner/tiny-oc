package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
)

func init() {
	watchCmd.Flags().Bool("json", false, "Output each step as newline-delimited JSON")
	watchCmd.Flags().BoolP("verbose", "v", false, "Show full assistant messages and thinking blocks")
	watchCmd.ValidArgsFunction = completeSessionIDs
	rootCmd.AddCommand(watchCmd)
}

var watchCmd = &cobra.Command{
	Use:   "watch <session-id>",
	Short: "Live-tail an agent session",
	Long:  "Stream formatted activity from a running agent session. Shows tool calls, skills, and text output in real time.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		s, err := session.FindByIDOrPrefix(args[0])
		if err != nil {
			return err
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		verbose, _ := cmd.Flags().GetBool("verbose")

		return runWatch(s, jsonFlag, verbose)
	},
}
