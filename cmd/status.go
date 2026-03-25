package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
	"github.com/tiny-oc/toc/internal/usage"
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
				if len(a.Skills) > 0 {
					fmt.Printf("      %s %s\n", ui.Dim("skills:"), ui.Dim(strings.Join(a.Skills, ", ")))
				}
			}
		}
		fmt.Println()

		// Skills
		locals, _ := skill.ListLocal()
		reg, _ := skill.LoadRegistry()
		totalSkills := len(locals) + len(reg.Skills)
		fmt.Printf("  %s", ui.Bold("Skills"))
		if totalSkills == 0 {
			fmt.Printf("  %s\n", ui.Dim("none"))
		} else {
			fmt.Printf(" %s\n", ui.Dim(fmt.Sprintf("(%d)", totalSkills)))
			for _, s := range locals {
				desc := s.Description
				if len(desc) > 50 {
					desc = desc[:47] + "..."
				}
				fmt.Printf("    %s %s %s\n", ui.Dim("▪"), ui.Cyan(s.Name), ui.Dim(desc))
			}
			for _, r := range reg.Skills {
				fmt.Printf("    %s %s %s\n", ui.Dim("▪"), ui.Cyan(r.Name), ui.Dim(r.URL))
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
				status := s.ResolvedStatus()
				var badge string
				switch status {
				case "active":
					badge = ui.Green("● active")
				case "completed":
					badge = ui.Dim("○ completed")
				case "stale":
					badge = ui.Yellow("◌ stale")
				}
				tokens := usage.ForSession(s.WorkspacePath, s.ID)
				tokenStr := tokens.FormatTotal()
				if tokenStr != "" {
					fmt.Printf("    %s  %s  %s  %s  %s\n", badge, ui.Cyan(s.Agent), ui.Dim(age), ui.Dim(s.ID[:8]), ui.Dim(tokenStr))
				} else {
					fmt.Printf("    %s  %s  %s  %s\n", badge, ui.Cyan(s.Agent), ui.Dim(age), ui.Dim(s.ID[:8]))
				}
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
