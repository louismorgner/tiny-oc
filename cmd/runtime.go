package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(runtimeCmd)
}

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Agent runtime commands — for use by agents during sessions",
	Long:  "Commands available to agents during a toc session. Requires TOC_WORKSPACE, TOC_AGENT, and TOC_SESSION_ID environment variables.",
}
