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
	Short: "Browse skills and agent templates in the registry",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Info("Fetching registry...")

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
			if query != "" {
				ui.Warn("No results for '%s'", query)
			} else {
				ui.Warn("Registry is empty")
			}
			return nil
		}

		fmt.Println()
		fmt.Printf("  %-20s %-8s %s\n", ui.Bold("NAME"), ui.Bold("TYPE"), ui.Bold("DESCRIPTION"))
		fmt.Printf("  %-20s %-8s %s\n", ui.Dim("────────────────────"), ui.Dim("────────"), ui.Dim("───────────────────────────────────────"))

		for _, e := range results {
			desc := e.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			typeBadge := ui.Dim(e.Type)
			switch e.Type {
			case "agent":
				extra := []string{e.Model}
				if len(e.Skills) > 0 {
					extra = append(extra, fmt.Sprintf("%d skill(s)", len(e.Skills)))
				}
				desc = fmt.Sprintf("%s %s", desc, ui.Dim("["+strings.Join(extra, ", ")+"]"))
			case "workspace":
				desc = fmt.Sprintf("%s %s", desc, ui.Dim(fmt.Sprintf("[%d agent(s)]", len(e.Agents))))
			}
			fmt.Printf("  %-20s %-8s %s\n", ui.Cyan(e.Name), typeBadge, desc)
		}

		fmt.Println()
		ui.Info("Install with: %s", ui.Bold("toc registry install <name>"))
		fmt.Println()
		return nil
	},
}
