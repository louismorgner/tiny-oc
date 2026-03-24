package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
}
