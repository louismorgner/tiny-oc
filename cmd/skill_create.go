package cmd

import (
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	skillCreateCmd.Flags().String("name", "", "skill name (skip interactive prompt)")
	skillCreateCmd.Flags().String("description", "", "skill description")
	skillCreateCmd.Flags().String("instructions", "", "skill instructions (or @file to read from file)")
	skillCmd.AddCommand(skillCreateCmd)
}

var skillCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new local skill interactively",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		instructionsFlag, _ := cmd.Flags().GetString("instructions")

		// If --name is provided, run in non-interactive mode
		if name != "" {
			if err := skill.ValidateName(name); err != nil {
				return err
			}
			if skill.Exists(name) {
				return fmt.Errorf("skill '%s' already exists", name)
			}
			if description == "" {
				return fmt.Errorf("--description is required in non-interactive mode")
			}

			instructions := instructionsFlag
			if strings.HasPrefix(instructions, "@") {
				data, err := os.ReadFile(strings.TrimPrefix(instructions, "@"))
				if err != nil {
					return fmt.Errorf("failed to read instructions file: %w", err)
				}
				instructions = string(data)
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

			ui.Success("Created skill %s", ui.Bold(name))
			return nil
		}

		// Interactive mode
		ui.Header("Create a new skill")

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
