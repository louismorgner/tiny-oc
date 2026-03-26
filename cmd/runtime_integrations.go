package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/runtime"
)

func init() {
	runtimeCmd.AddCommand(runtimeIntegrationsCmd)
}

var runtimeIntegrationsCmd = &cobra.Command{
	Use:   "integrations",
	Short: "List configured integrations and permitted actions for the current agent",
	Long: `List all integrations that are configured in the workspace and show which
actions the current agent is permitted to use.

This command reads the session permission manifest to determine what the
running agent can access. Each integration shows its configuration status
and the permitted actions with their scope restrictions.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		manifest, err := loadPermissionManifest(ctx)
		if err != nil {
			return fmt.Errorf("failed to load permission manifest: %w", err)
		}

		if len(manifest.Integrations) == 0 {
			fmt.Println("No integrations configured for this agent.")
			fmt.Println("Add one with: toc integrate add <name>")
			return nil
		}

		// Sort integration names for stable output
		names := make([]string, 0, len(manifest.Integrations))
		for name := range manifest.Integrations {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			perms := manifest.Integrations[name]

			// Check if credentials exist
			configured := integration.CredentialExistsInWorkspace(ctx.Workspace, name)
			status := "configured"
			if !configured {
				status = "no credentials"
			}

			fmt.Printf("%s (%s)\n", name, status)

			if len(perms) == 0 {
				fmt.Println("    (no permissions)")
				continue
			}

			for _, grant := range perms {
				fmt.Printf("    %s\n", grant.String())
			}
		}

		return nil
	},
}
