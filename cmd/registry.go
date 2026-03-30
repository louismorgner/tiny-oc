package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(registryCmd)
}

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Browse skills, agents, and workspaces in the registry",
}
