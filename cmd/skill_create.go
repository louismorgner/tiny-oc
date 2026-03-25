package cmd

import (
	"fmt"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	skillCmd.AddCommand(skillCreateCmd)
}

var skillCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new local skill interactively",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		ui.Header("Create a new skill")

		var name, description string
		var instructions string

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Skill name").
					Description("lowercase letters, numbers, and hyphens (e.g. 'code-review')").
					Value(&name).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("name is required")
						}
						if err := skill.ValidateName(s); err != nil {
							return err
						}
						if skill.Exists(s) {
							return fmt.Errorf("skill '%s' already exists", s)
						}
						return nil
					}),

				huh.NewInput().
					Title("Description").
					Description("what does this skill do and when should it be used?").
					Value(&description).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("description is required")
						}
						if len(s) > 1024 {
							return fmt.Errorf("description must be under 1024 characters")
						}
						return nil
					}),
			),

			huh.NewGroup(
				huh.NewText().
					Title("Instructions").
					Description("the skill's instructions — loaded when the skill is activated.\nyou can always edit SKILL.md later (optional, press enter to skip)").
					Value(&instructions),
			),
		)

		form.WithProgramOptions(ui.FormOptions()...)
		if err := form.Run(); err != nil {
			return err
		}

		meta := skill.SkillMeta{
			Name:        name,
			Description: description,
		}

		body := ""
		if strings.TrimSpace(instructions) != "" {
			body = strings.TrimSpace(instructions)
		}

		if err := skill.CreateFromMeta(meta, body); err != nil {
			return err
		}

		auditLog("skill.create", map[string]interface{}{
			"skill": name,
		})

		fmt.Println()
		ui.Success("Created skill %s", ui.Bold(name))
		ui.Info("SKILL.md: %s", ui.Dim(skill.Dir(name)+"/SKILL.md"))
		if body == "" {
			ui.Info("Edit %s to add instructions.", ui.Bold(skill.Dir(name)+"/SKILL.md"))
		}
		fmt.Println()
		return nil
	},
}
