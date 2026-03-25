package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(integrateCmd)
}

var integrateCmd = &cobra.Command{
	Use:   "integrate",
	Short: "Manage external integrations (GitHub, Slack, etc.)",
	Long:  "Add, remove, and test external tool integrations. Credentials are encrypted at rest with the master key stored in the OS keychain.",
}
