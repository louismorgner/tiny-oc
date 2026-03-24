package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(registryCmd)
}

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Browse the skills and agent registry",
}
