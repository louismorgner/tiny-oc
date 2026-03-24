package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/spawn"
)

var resumeSessionID string

func init() {
	agentSpawnCmd.Flags().StringVar(&resumeSessionID, "resume", "", "Resume an existing session by ID")
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
			result, err := spawn.ResumeSession(s)
			if result != nil {
				details := map[string]interface{}{
					"agent":      agentName,
					"session_id": resumeSessionID,
				}
				if result.SyncedFiles > 0 {
					details["files_synced"] = result.SyncedFiles
				}
				_ = audit.Log("session.resume", details)
			}
			return err
		}

		cfg, err := agent.Load(agentName)
		if err != nil {
			return err
		}

		result, err := spawn.SpawnSession(cfg)
		if result != nil {
			details := map[string]interface{}{
				"agent":      agentName,
				"session_id": result.SessionID,
			}
			if result.SyncedFiles > 0 {
				details["files_synced"] = result.SyncedFiles
			}
			_ = audit.Log("session.spawn", details)
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
