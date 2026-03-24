package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(skillCmd)
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage skills",
}
