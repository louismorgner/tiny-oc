package cmd

import (
	"fmt"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
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
		var contextRaw, skillsRaw, instructions string
		var onEnd, composeRaw, subAgentsRaw string

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
					Description("what does this agent do? (optional)").
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
					Title("Skills").
					Description("skill names or URLs — one per line (optional).\nuse 'toc skill list' to see available skills.").
					Value(&skillsRaw),

				huh.NewText().
					Title("Context sync patterns").
					Description("files synced back from sessions to the agent template.\none per line, e.g. context/*.md, docs/ (optional)").
					Value(&contextRaw),

				huh.NewText().
					Title("Agent instructions").
					Description("loaded as context when you spawn a session.\nyou can always edit agent.md later (optional)").
					Value(&instructions),
			),

			huh.NewGroup(
				huh.NewText().
					Title("Compose files").
					Description("additional markdown files appended after agent.md.\none per line, e.g. soul.md, user.md (optional)").
					Value(&composeRaw),

				huh.NewText().
					Title("Sub-agents").
					Description("agents this agent can spawn.\none per line, or * to allow all (optional)").
					Value(&subAgentsRaw),

				huh.NewText().
					Title("On-end hook").
					Description("prompt sent to the agent when a session ends.\ne.g. Save summary to context/notes.md (optional)").
					Value(&onEnd),
			),
		)

		basics.WithProgramOptions(ui.FormOptions()...)
		if err := basics.Run(); err != nil {
			return err
		}

		// Parse multiline inputs
		parseLines := func(raw string) []string {
			var result []string
			if raw != "" {
				for _, line := range strings.Split(raw, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						result = append(result, line)
					}
				}
			}
			return result
		}

		contextPatterns := parseLines(contextRaw)
		skills := parseLines(skillsRaw)
		compose := parseLines(composeRaw)
		subAgents := parseLines(subAgentsRaw)

		var perms *agent.Permissions
		if len(subAgents) > 0 {
			perms = &agent.Permissions{
				SubAgents: make(map[string]agent.PermissionLevel),
			}
			for _, sa := range subAgents {
				perms.SubAgents[sa] = agent.PermOn
			}
		}

		cfg := agent.AgentConfig{
			Runtime:     "claude-code",
			Name:        name,
			Description: desc,
			Model:       model,
			Context:     contextPatterns,
			Skills:      skills,
			Perms:       perms,
			OnEnd:       strings.TrimSpace(onEnd),
			Compose:     compose,
		}

		agentMD := ""
		if strings.TrimSpace(instructions) != "" {
			agentMD = strings.TrimSpace(instructions)
		}

		if err := agent.CreateWithInstructions(cfg, agentMD); err != nil {
			return err
		}

		auditLog("agent.create", map[string]interface{}{
			"agent":   name,
			"model":   model,
			"runtime": "claude-code",
		})

		fmt.Println()
		ui.Success("Created agent %s", ui.Bold(name))
		ui.Info("Config: %s", ui.Dim(agent.Dir(name)+"/oc-agent.yaml"))
		if len(skills) > 0 {
			ui.Info("Skills: %s", ui.Dim(fmt.Sprintf("%d skill(s)", len(skills))))
		}
		if len(contextPatterns) > 0 {
			ui.Info("Context sync: %s", ui.Dim(fmt.Sprintf("%d pattern(s)", len(contextPatterns))))
		}
		if len(compose) > 0 {
			ui.Info("Compose: %s", ui.Dim(fmt.Sprintf("%d file(s)", len(compose))))
		}
		if len(subAgents) > 0 {
			ui.Info("Sub-agents: %s", ui.Dim(strings.Join(subAgents, ", ")))
		}
		if cfg.OnEnd != "" {
			ui.Info("On-end hook: %s", ui.Dim("configured"))
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
