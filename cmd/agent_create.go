package cmd

import (
	"fmt"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	agentCmd.AddCommand(agentCreateCmd)
}

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new agent interactively",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		ui.Header("Create a new agent")

		var name, desc, model string
		var contextRaw, instructions string

		basics := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Agent name").
					Description("lowercase letters, numbers, and hyphens").
					Value(&name).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("name is required")
						}
						if err := agent.ValidateName(s); err != nil {
							return err
						}
						if agent.Exists(s) {
							return fmt.Errorf("agent '%s' already exists", s)
						}
						return nil
					}),

				huh.NewInput().
					Title("Description").
					Description("what does this agent do? (optional, press enter to skip)").
					Value(&desc),

				huh.NewSelect[string]().
					Title("Model").
					Options(
						huh.NewOption("Sonnet — fast, great for most tasks", "sonnet"),
						huh.NewOption("Opus — most capable, deeper reasoning", "opus"),
						huh.NewOption("Haiku — lightweight, quick responses", "haiku"),
					).
					Value(&model),
			),

			huh.NewGroup(
				huh.NewText().
					Title("Context sync patterns").
					Description("files matching these patterns sync back from sessions to the agent template.\none per line, e.g. context/*.md, docs/, notes.txt (optional, press enter to skip)").
					Value(&contextRaw),

				huh.NewText().
					Title("Agent instructions").
					Description("initial instructions for this agent — loaded as context when you spawn a session.\nyou can always edit agent.md later (optional, press enter to skip)").
					Value(&instructions),
			),
		)

		if err := basics.Run(); err != nil {
			return err
		}

		// Parse context patterns from multiline input
		var contextPatterns []string
		if contextRaw != "" {
			for _, line := range strings.Split(contextRaw, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					contextPatterns = append(contextPatterns, line)
				}
			}
		}

		cfg := agent.AgentConfig{
			Runtime:     "claude-code",
			Name:        name,
			Description: desc,
			Model:       model,
			Context:     contextPatterns,
		}

		agentMD := ""
		if strings.TrimSpace(instructions) != "" {
			agentMD = strings.TrimSpace(instructions)
		}

		if err := agent.CreateWithInstructions(cfg, agentMD); err != nil {
			return err
		}

		_ = audit.Log("agent.create", map[string]interface{}{
			"agent":   name,
			"model":   model,
			"runtime": "claude-code",
		})

		fmt.Println()
		ui.Success("Created agent %s", ui.Bold(name))
		ui.Info("Config: %s", ui.Dim(agent.Dir(name)+"/oc-agent.yaml"))
		if len(contextPatterns) > 0 {
			ui.Info("Context sync: %s", ui.Dim(fmt.Sprintf("%d pattern(s)", len(contextPatterns))))
		}
		if agentMD != "" {
			ui.Info("Instructions written to %s", ui.Dim(agent.Dir(name)+"/agent.md"))
		} else {
			ui.Info("Edit %s to add instructions.", ui.Bold(agent.Dir(name)+"/agent.md"))
		}
		fmt.Println()
		ui.Info("Spawn it with: %s", ui.Bold("toc agent spawn "+name))
		fmt.Println()
		return nil
	},
}
