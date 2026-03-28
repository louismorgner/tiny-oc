package cmd

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/usage"
)

const refreshInterval = 2 * time.Second

type tickMsg time.Time

type statusModel struct {
	cfg      *config.WorkspaceConfig
	agents   []agent.AgentConfig
	sessions []session.Session
	err      error
	width    int
	height   int
	quitting bool
}

func initialModel(cfg *config.WorkspaceConfig) statusModel {
	m := statusModel{cfg: cfg}
	m.refresh()
	return m
}

func (m *statusModel) refresh() {
	agents, err := agent.List()
	if err != nil {
		m.err = err
		return
	}
	m.agents = agents

	sf, err := session.Load()
	if err != nil {
		m.err = err
		return
	}
	m.sessions = sf.Sessions
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m statusModel) Init() tea.Cmd {
	return tickCmd()
}

func (m statusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.refresh()
		return m, tickCmd()
	}
	return m, nil
}

func (m statusModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	if m.err != nil {
		return tea.NewView(fmt.Sprintf("  Error: %v\n", m.err))
	}

	var b strings.Builder

	dim := lipgloss.NewStyle().Faint(true).Render
	bold := lipgloss.NewStyle().Bold(true).Render
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s %s\n", bold("Workspace:"), cyan(m.cfg.Name)))
	b.WriteString(fmt.Sprintf("  %s %s\n", bold("Config:"), dim(config.TocDir()+"/")))
	b.WriteString("\n")

	// Build agent name → model map for cost lookups.
	agentModel := make(map[string]string, len(m.agents))
	for _, a := range m.agents {
		agentModel[a.Name] = a.Model
	}

	// Token totals per agent
	agentTokens := make(map[string]usage.TokenUsage)
	var totalTokens usage.TokenUsage
	for _, s := range m.sessions {
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
	b.WriteString(fmt.Sprintf("  %s", bold("Agents")))
	if len(m.agents) == 0 {
		b.WriteString(fmt.Sprintf("  %s\n", dim("none")))
	} else {
		totalStr := totalTokens.FormatTotal()
		if totalStr != "" {
			b.WriteString(fmt.Sprintf(" %s %s\n", dim(fmt.Sprintf("(%d)", len(m.agents))), dim("— "+totalStr+" total")))
		} else {
			b.WriteString(fmt.Sprintf(" %s\n", dim(fmt.Sprintf("(%d)", len(m.agents)))))
		}
		for _, a := range m.agents {
			problems := a.Validate()
			if len(problems) == 0 {
				desc := ""
				if a.Description != "" {
					desc = " " + dim("— "+a.Description)
				}
				agTok := agentTokens[a.Name]
				tokenStr := agTok.FormatTotal()
				if tokenStr != "" {
					costStr := usage.FormatCost(usage.EstimateCost(a.Model, agTok))
					bracket := tokenStr
					if costStr != "" {
						bracket += "  " + costStr
					}
					desc += " " + dim("["+bracket+"]")
				}
				b.WriteString(fmt.Sprintf("    %s %s %s%s\n", green("✓"), cyan(a.Name), dim(a.Model), desc))
			} else {
				b.WriteString(fmt.Sprintf("    %s %s %s\n", red("✗"), cyan(a.Name), red(strings.Join(problems, ", "))))
			}
		}
	}
	b.WriteString("\n")

	// Sessions
	b.WriteString(fmt.Sprintf("  %s", bold("Sessions")))
	if len(m.sessions) == 0 {
		b.WriteString(fmt.Sprintf("  %s\n", dim("none")))
	} else {
		active, completed, failed := 0, 0, 0
		for i := range m.sessions {
			switch m.sessions[i].ResolvedStatus() {
			case "active":
				active++
			case "completed", session.StatusCompletedOK:
				completed++
			case session.StatusCompletedError, session.StatusZombie:
				failed++
			}
		}
		summary := fmt.Sprintf("(%d total", len(m.sessions))
		if active > 0 {
			summary += fmt.Sprintf(", %d active", active)
		}
		if failed > 0 {
			summary += fmt.Sprintf(", %d failed", failed)
		}
		summary += ")"
		b.WriteString(fmt.Sprintf(" %s\n", dim(summary)))

		shown := make([]session.Session, len(m.sessions))
		copy(shown, m.sessions)
		sortSessions(shown)

		limit := 15
		if len(shown) < limit {
			limit = len(shown)
		}
		shown = shown[:limit]

		for _, s := range shown {
			age := timeAgo(s.CreatedAt)
			status := s.ResolvedStatus()
			var badge string
			switch status {
			case "active":
				badge = green("● active   ")
			case "completed", session.StatusCompletedOK:
				badge = dim("○ completed")
			case session.StatusCompletedError:
				badge = red("✗ error    ")
			case session.StatusZombie:
				badge = red("◌ zombie   ")
			case session.StatusCancelled:
				badge = yellow("◌ cancelled")
			case "stale":
				badge = yellow("◌ stale    ")
			default:
				badge = dim("○ " + status + strings.Repeat(" ", maxInt(0, 9-len(status))))
			}

			idStr := s.ID
			if len(idStr) > 8 {
				idStr = idStr[:8]
			}

			parent := ""
			if s.ParentSessionID != "" {
				pid := s.ParentSessionID
				if len(pid) > 8 {
					pid = pid[:8]
				}
				parent = dim(" ← " + pid)
			}

			tokens := usage.ForSession(&s)
			tokenStr := tokens.FormatTotal()
			tokenCol := ""
			if tokenStr != "" {
				costStr := usage.FormatCost(usage.EstimateCost(agentModel[s.Agent], tokens))
				if costStr != "" {
					tokenCol = "  " + dim(tokenStr+"  "+costStr)
				} else {
					tokenCol = "  " + dim(tokenStr)
				}
			}

			nameCol := ""
			if s.Name != "" {
				nameCol = "  " + cyan(s.Name)
			}

			b.WriteString(fmt.Sprintf("    %s  %-12s  %-14s  %s%s%s%s\n",
				badge, cyan(s.Agent), dim(age), dim(idStr), nameCol, parent, tokenCol))
		}
		if len(m.sessions) > limit {
			b.WriteString(fmt.Sprintf("    %s\n", dim(fmt.Sprintf("... and %d more", len(m.sessions)-limit))))
		}
	}
	b.WriteString("\n")

	// Footer
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		dim("Refreshing every 2s"),
		dim("Press q to quit")))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
