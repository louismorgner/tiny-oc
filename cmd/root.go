package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	audit.SetVersion(version)
}

var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "toc",
	Short:   "tiny-oc — local agent workspace manager",
	Version: version,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Suggest agent names as root-level shorthand completions
		if len(args) != 0 || !config.Exists() {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		agents, err := agent.List()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var completions []string
		for _, a := range agents {
			desc := "spawn agent"
			if a.Description != "" {
				desc = a.Description
			}
			completions = append(completions, fmt.Sprintf("%s\t%s", a.Name, desc))
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	},
}

// knownCommands returns the set of registered root-level command names.
func knownCommands() map[string]bool {
	cmds := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		cmds[c.Name()] = true
		for _, alias := range c.Aliases {
			cmds[alias] = true
		}
	}
	// Built-in cobra commands
	cmds["help"] = true
	cmds["completion"] = true
	return cmds
}

// auditLog wraps audit.Log and warns on failure rather than silently discarding errors.
func auditLog(action string, details map[string]interface{}) {
	if err := audit.Log(action, details); err != nil {
		ui.Warn("audit log failed: %s", err)
	}
}

func Execute() {
	// Shorthand: `toc <agent-name>` → `toc agent spawn <agent-name>`
	if len(os.Args) >= 2 {
		firstArg := os.Args[1]
		// Skip flags, known commands, and help/version
		if firstArg != "" && firstArg[0] != '-' && !knownCommands()[firstArg] {
			// Check if this looks like an agent name
			if config.Exists() {
				if _, err := agent.Load(firstArg); err == nil {
					// Rewrite args: toc <name> [flags] → toc agent spawn <name> [flags]
					newArgs := []string{os.Args[0], "agent", "spawn"}
					newArgs = append(newArgs, os.Args[1:]...)
					os.Args = newArgs
				}
			}
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
