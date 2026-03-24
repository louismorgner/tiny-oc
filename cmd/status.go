package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace overview",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.EnsureInitialized()
		if err != nil {
			return err
		}

		agents, err := agent.List()
		if err != nil {
			return err
		}

		sf, err := session.Load()
		if err != nil {
			return err
		}

		fmt.Println()
		fmt.Printf("  %s %s\n", ui.Bold("Workspace:"), ui.Cyan(cfg.Name))
		fmt.Printf("  %s %s\n", ui.Bold("Config:"), ui.Dim(config.TocDir()+"/"))
		fmt.Println()

		// Agents
		fmt.Printf("  %s", ui.Bold("Agents"))
		if len(agents) == 0 {
			fmt.Printf("  %s\n", ui.Dim("none"))
			fmt.Printf("  %s\n", ui.Dim("Run 'toc agent create' to get started."))
		} else {
			fmt.Printf(" %s\n", ui.Dim(fmt.Sprintf("(%d)", len(agents))))
			for _, a := range agents {
				problems := a.Validate()
				if len(problems) == 0 {
					desc := ""
					if a.Description != "" {
						desc = " " + ui.Dim("— "+a.Description)
					}
					fmt.Printf("    %s %s %s%s\n", ui.Green("✓"), ui.Cyan(a.Name), ui.Dim(a.Model), desc)
				} else {
					fmt.Printf("    %s %s %s\n", ui.Red("✗"), ui.Cyan(a.Name), ui.Red(strings.Join(problems, ", ")))
				}
			}
		}
		fmt.Println()

		// Recent sessions
		fmt.Printf("  %s", ui.Bold("Recent sessions"))
		if len(sf.Sessions) == 0 {
			fmt.Printf("  %s\n", ui.Dim("none"))
		} else {
			shown := sf.Sessions
			if len(shown) > 5 {
				shown = shown[len(shown)-5:]
			}
			fmt.Printf(" %s\n", ui.Dim(fmt.Sprintf("(%d total)", len(sf.Sessions))))
			for _, s := range shown {
				age := timeAgo(s.CreatedAt)
				fmt.Printf("    %s %s  %s  %s\n", ui.Dim("▪"), ui.Cyan(s.Agent), ui.Dim(age), ui.Dim(s.ID[:8]))
			}
		}
		fmt.Println()

		return nil
	},
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
