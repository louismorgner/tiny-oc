package cmd

import (
	"fmt"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	agentCmd.AddCommand(agentSkillsCmd)
}

var agentSkillsCmd = &cobra.Command{
	Use:               "skills <agent-name>",
	Short:             "Manage skills for an agent",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAgentNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]
		cfg, err := agent.Load(name)
		if err != nil {
			return err
		}

		// Build list of available skills (local + url refs)
		locals, _ := skill.ListLocal()
		reg, _ := skill.LoadRegistry()

		// Build a set of currently enabled skills for pre-selection
		enabled := make(map[string]bool)
		for _, s := range cfg.Skills {
			enabled[s] = true
		}

		var options []huh.Option[string]
		for _, s := range locals {
			label := fmt.Sprintf("%s — %s (local)", s.Name, truncate(s.Description, 50))
			options = append(options, huh.NewOption(label, s.Name))
		}
		for _, r := range reg.Skills {
			label := fmt.Sprintf("%s — %s (url)", r.Name, r.URL)
			options = append(options, huh.NewOption(label, r.Name))
		}

		if len(options) == 0 {
			ui.Warn("No skills available. Run %s or %s first.", ui.Bold("toc skill create"), ui.Bold("toc skill add <url>"))
			return nil
		}

		// Pre-populate with currently enabled skills so huh pre-selects them
		selected := make([]string, len(cfg.Skills))
		copy(selected, cfg.Skills)

		ui.Header(fmt.Sprintf("Skills for %s", ui.Bold(name)))

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select skills to enable").
					Value(&selected).
					Height(len(options)+3).
					Options(options...),
			),
		)

		form.WithProgramOptions(ui.FormOptions()...)
		if err := form.Run(); err != nil {
			return err
		}

		cfg.Skills = selected
		if err := agent.Save(cfg); err != nil {
			return err
		}

		auditLog("agent.skills.update", map[string]interface{}{
			"agent":  name,
			"skills": strings.Join(selected, ", "),
		})

		fmt.Println()
		if len(selected) == 0 {
			ui.Success("Cleared all skills for %s", ui.Bold(name))
		} else {
			ui.Success("Updated skills for %s: %s", ui.Bold(name), ui.Dim(strings.Join(selected, ", ")))
		}
		fmt.Println()
		return nil
	},
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}
