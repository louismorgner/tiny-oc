package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	skillCmd.AddCommand(skillRemoveCmd)
}

var skillRemoveCmd = &cobra.Command{
	Use:               "remove <name>",
	Short:             "Remove a skill",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeSkillNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]

		// Determine if it's a local skill or URL reference
		isLocal := skill.Exists(name)
		_, refErr := skill.FindRef(name)
		isRef := refErr == nil

		if !isLocal && !isRef {
			return fmt.Errorf("skill '%s' not found", name)
		}

		skillType := "local"
		if !isLocal && isRef {
			skillType = "url"
		}

		confirm, err := ui.Prompt(fmt.Sprintf("Remove %s skill %s? (y/N)", skillType, ui.Bold(name)), "n")
		if err != nil {
			return err
		}
		if confirm != "y" && confirm != "Y" {
			ui.Info("Cancelled.")
			return nil
		}

		if isLocal {
			if err := skill.Remove(name); err != nil {
				return err
			}
		}
		if isRef {
			if err := skill.RemoveRef(name); err != nil {
				return err
			}
		}

		auditLog("skill.remove", map[string]interface{}{
			"skill": name,
			"type":  skillType,
		})

		fmt.Println()
		ui.Success("Removed skill %s", ui.Bold(name))
		fmt.Println()
		return nil
	},
}

func completeSkillNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 || !config.Exists() {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string

	locals, _ := skill.ListLocal()
	for _, s := range locals {
		completions = append(completions, fmt.Sprintf("%s\t%s (local)", s.Name, s.Description))
	}

	reg, _ := skill.LoadRegistry()
	for _, r := range reg.Skills {
		completions = append(completions, fmt.Sprintf("%s\t%s (url)", r.Name, r.URL))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
