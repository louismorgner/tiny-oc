package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/spawn"
)

var (
	resumeSessionID    string
	spawnPrompt        string
	spawnMaxIterations int
	spawnInspect       bool
)

func init() {
	agentSpawnCmd.Flags().StringVar(&resumeSessionID, "resume", "", "Resume an existing session by ID")
	agentSpawnCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "Run a single prompt non-interactively and exit")
	agentSpawnCmd.Flags().IntVar(&spawnMaxIterations, "max-iterations", 0, "Override max tool iterations for this session (0 = use agent/env/default)")
	agentSpawnCmd.Flags().BoolVar(&spawnInspect, "inspect", false, "Capture full upstream LLM request/response traffic for this session")
	agentSpawnCmd.RegisterFlagCompletionFunc("resume", completeSessionIDs)
	agentCmd.AddCommand(agentSpawnCmd)
}

var agentSpawnCmd = &cobra.Command{
	Use:               "spawn <agent-name>",
	Short:             "Spawn a new agent session or resume an existing one",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAgentNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		agentName := args[0]

		if resumeSessionID != "" {
			s, err := session.FindByID(resumeSessionID)
			if err != nil {
				return err
			}
			if s.Agent != agentName {
				return fmt.Errorf("session '%s' belongs to agent '%s', not '%s'", resumeSessionID, s.Agent, agentName)
			}
			result, err := spawn.ResumeSession(s, spawn.ResumeOptions{Inspect: spawnInspect})
			if result != nil {
				details := map[string]interface{}{
					"agent":      agentName,
					"session_id": resumeSessionID,
					"inspect":    spawnInspect,
				}
				if result.SyncedFiles > 0 {
					details["files_synced"] = result.SyncedFiles
				}
				auditLog("session.resume", details)
			}
			return err
		}

		cfg, err := agent.Load(agentName)
		if err != nil {
			return err
		}

		result, err := spawn.SpawnSession(cfg, spawn.SpawnOptions{
			Prompt:        spawnPrompt,
			MaxIterations: spawnMaxIterations,
			Inspect:       spawnInspect,
		})
		if result != nil {
			details := map[string]interface{}{
				"agent":      agentName,
				"session_id": result.SessionID,
				"inspect":    spawnInspect,
			}
			if result.SyncedFiles > 0 {
				details["files_synced"] = result.SyncedFiles
			}
			auditLog("session.spawn", details)
		}
		return err
	},
}

func completeAgentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if !config.Exists() {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	agents, err := agent.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var completions []string
	for _, a := range agents {
		if a.Description != "" {
			completions = append(completions, fmt.Sprintf("%s\t%s", a.Name, a.Description))
		} else {
			completions = append(completions, a.Name)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func completeSessionIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if !config.Exists() {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var agentName string
	if len(args) > 0 {
		agentName = args[0]
	}

	sf, err := session.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var completions []string
	for _, s := range sf.Sessions {
		if agentName != "" && s.Agent != agentName {
			continue
		}
		completions = append(completions, fmt.Sprintf("%s\t%s (%s)", s.ID, s.Agent, s.CreatedAt.Format("2006-01-02 15:04")))
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
