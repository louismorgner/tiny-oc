package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/registry"
	"github.com/tiny-oc/toc/internal/ui"
)

var registrySearchJSON bool

func init() {
	registrySearchCmd.Flags().BoolVar(&registrySearchJSON, "json", false, "Output as JSON")
	registryCmd.AddCommand(registrySearchCmd)
}

var registrySearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for skills in the registry",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Info("Fetching registry index...")

		index, err := registry.FetchIndex()
		if err != nil {
			return err
		}

		query := ""
		if len(args) > 0 {
			query = args[0]
		}

		results := registry.Search(index, query)

		if registrySearchJSON {
			out, err := registry.FormatJSON(results)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		}

		if len(results) == 0 {
			ui.Warn("No skills found matching '%s'", query)
			return nil
		}

		fmt.Println()
		fmt.Printf("  %-20s %-12s %s\n", ui.Bold("NAME"), ui.Bold("TAGS"), ui.Bold("DESCRIPTION"))
		fmt.Printf("  %-20s %-12s %s\n", ui.Dim("────────────────────"), ui.Dim("────────────"), ui.Dim("───────────────────────────────"))

		for _, s := range results {
			tags := ""
			if len(s.Tags) > 0 {
				tags = strings.Join(s.Tags, ", ")
			}
			desc := s.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			fmt.Printf("  %-20s %-12s %s\n", ui.Cyan(s.Name), ui.Dim(tags), desc)
		}

		fmt.Println()
		ui.Info("Install with: %s", ui.Bold("toc skill add --registry <name>"))
		fmt.Println()
		return nil
	},
}
