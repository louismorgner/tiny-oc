package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	skillCmd.AddCommand(skillListCmd)
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		locals, err := skill.ListLocal()
		if err != nil {
			return err
		}

		reg, err := skill.LoadRegistry()
		if err != nil {
			return err
		}

		if len(locals) == 0 && len(reg.Skills) == 0 {
			ui.Warn("No skills configured. Run %s or %s to add one.", ui.Bold("toc skill create <name>"), ui.Bold("toc skill add <url>"))
			return nil
		}

		fmt.Println()
		fmt.Printf("  %-20s %-8s %s\n", ui.Bold("NAME"), ui.Bold("TYPE"), ui.Bold("DESCRIPTION"))
		fmt.Printf("  %-20s %-8s %s\n", ui.Dim("────────────────────"), ui.Dim("────────"), ui.Dim("───────────────────────────────"))

		for _, s := range locals {
			desc := s.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			fmt.Printf("  %-20s %-8s %s\n", ui.Cyan(s.Name), "local", ui.Dim(desc))
		}

		for _, r := range reg.Skills {
			fmt.Printf("  %-20s %-8s %s\n", ui.Cyan(r.Name), "url", ui.Dim(r.URL))
		}

		fmt.Println()
		return nil
	},
}
