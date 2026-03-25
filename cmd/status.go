package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
	"github.com/tiny-oc/toc/internal/usage"
)

func init() {
	statusCmd.Flags().Bool("static", false, "Print once and exit (no live updates)")
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace overview",
	Long:  "Show a live-updating dashboard of workspace status. Use --static for a single snapshot.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.EnsureInitialized()
		if err != nil {
			return err
		}

		static, _ := cmd.Flags().GetBool("static")

		// Use interactive TUI unless --static is set or stdout is not a terminal
		if !static && isTerminal() {
			m := initialModel(cfg)
			p := tea.NewProgram(m)
			_, err := p.Run()
			return err
		}

		return printStaticStatus(cfg)
	},
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func printStaticStatus(cfg *config.WorkspaceConfig) error {
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

	// Build per-agent token totals
	agentTokens := make(map[string]usage.TokenUsage)
	var totalTokens usage.TokenUsage
	for _, s := range sf.Sessions {
		tokens := usage.ForSession(&s)
		combined := agentTokens[s.Agent]
		combined.InputTokens += tokens.InputTokens
		combined.OutputTokens += tokens.OutputTokens
		combined.CacheRead += tokens.CacheRead
		combined.CacheCreate += tokens.CacheCreate
		agentTokens[s.Agent] = combined
		totalTokens.InputTokens += tokens.InputTokens
		totalTokens.OutputTokens += tokens.OutputTokens
		totalTokens.CacheRead += tokens.CacheRead
		totalTokens.CacheCreate += tokens.CacheCreate
	}

	// Agents
	fmt.Printf("  %s", ui.Bold("Agents"))
	if len(agents) == 0 {
		fmt.Printf("  %s\n", ui.Dim("none"))
		fmt.Printf("  %s\n", ui.Dim("Run 'toc agent create' to get started."))
	} else {
		totalStr := totalTokens.FormatTotal()
		if totalStr != "" {
			fmt.Printf(" %s %s\n", ui.Dim(fmt.Sprintf("(%d)", len(agents))), ui.Dim("— "+totalStr+" total"))
		} else {
			fmt.Printf(" %s\n", ui.Dim(fmt.Sprintf("(%d)", len(agents))))
		}
		for _, a := range agents {
			problems := a.Validate()
			if len(problems) == 0 {
				desc := ""
				if a.Description != "" {
					desc = " " + ui.Dim("— "+a.Description)
				}
				tokenStr := agentTokens[a.Name].FormatTotal()
				if tokenStr != "" {
					desc += " " + ui.Dim("["+tokenStr+"]")
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
		shown := make([]session.Session, len(sf.Sessions))
		copy(shown, sf.Sessions)
		sortSessions(shown)
		if len(shown) > 5 {
			shown = shown[:5]
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
			tokens := usage.ForSession(&s)
			tokenStr := tokens.FormatTotal()
			nameStr := ""
			if s.Name != "" {
				nameStr = "  " + ui.Cyan(s.Name)
			}
			if tokenStr != "" {
				fmt.Printf("    %s  %s  %s  %s%s  %s\n", badge, ui.Cyan(s.Agent), ui.Dim(age), ui.Dim(s.ID[:8]), nameStr, ui.Dim(tokenStr))
			} else {
				fmt.Printf("    %s  %s  %s  %s%s\n", badge, ui.Cyan(s.Agent), ui.Dim(age), ui.Dim(s.ID[:8]), nameStr)
			}
		}
	}
	fmt.Println()

	return nil
}

// sortSessions sorts active/running sessions first, then by most recent.
func sortSessions(sessions []session.Session) {
	sort.Slice(sessions, func(i, j int) bool {
		ai := sessions[i].ResolvedStatus() == "active"
		aj := sessions[j].ResolvedStatus() == "active"
		if ai != aj {
			return ai
		}
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
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
