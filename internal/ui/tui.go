package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/mattn/go-isatty"
)

// IsTTY returns true if the given file is a terminal.
func IsTTY(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// --- Styles (adaptive: work on light and dark terminals) ---

var (
	// Agent header bar
	agentHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("6")). // cyan
				PaddingLeft(1)

	// Session info line
	sessionInfoStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")). // dim gray
				PaddingLeft(1)

	// User prompt marker
	promptMarkerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("2")) // green

	// Separator between turns
	turnSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8"))

	// Tool call header
	toolHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	// Tool call name
	toolNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("3")) // yellow

	// Tool output preview
	toolOutputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			PaddingLeft(4)

	// Spinner/thinking indicator
	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")). // magenta
			Italic(true)

	// Error display
	errorBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")). // red
			Bold(true)

	// Timestamp style
	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

// --- Markdown Rendering ---

// markdownRenderer is lazily initialized.
var mdRenderer *glamour.TermRenderer

func getMarkdownRenderer() *glamour.TermRenderer {
	if mdRenderer != nil {
		return mdRenderer
	}
	style := glamour.AutoStyle
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		// Fallback: try dark style
		r, err = glamour.NewTermRenderer(
			glamour.WithStandardStyle(style),
			glamour.WithWordWrap(80),
		)
		if err != nil {
			return nil
		}
	}
	mdRenderer = r
	return mdRenderer
}

// RenderMarkdown renders markdown text for terminal display.
// Falls back to plain text if rendering fails or stdout is not a TTY.
func RenderMarkdown(text string) string {
	if !IsTTY(os.Stdout) {
		return text
	}
	r := getMarkdownRenderer()
	if r == nil {
		return text
	}
	rendered, err := r.Render(text)
	if err != nil {
		return text
	}
	// glamour adds trailing newlines; trim excess
	return strings.TrimRight(rendered, "\n")
}

// --- Session UI Components ---

// SessionBanner prints a styled banner when a native session starts.
func SessionBanner(agentName, sessionID, model string) string {
	var b strings.Builder

	// Top border
	b.WriteString("\n")

	// Agent name header
	header := fmt.Sprintf("  %s  %s", agentHeaderStyle.Render(agentName), sessionInfoStyle.Render(shortSessionID(sessionID)))
	b.WriteString(header)
	b.WriteString("\n")

	// Model and session info
	info := fmt.Sprintf("  %s  %s", sessionInfoStyle.Render("model:"), sessionInfoStyle.Render(model))
	b.WriteString(info)
	b.WriteString("\n")

	// Separator
	b.WriteString(TurnSeparator())
	b.WriteString("\n")

	return b.String()}

// TurnSeparator returns a visual separator between conversation turns.
func TurnSeparator() string {
	return turnSeparatorStyle.Render("  " + strings.Repeat("─", 60))
}

// UserPromptPrefix returns the styled prompt prefix for user input.
func UserPromptPrefix(agentName string) string {
	return promptMarkerStyle.Render(agentName) + promptMarkerStyle.Render(" > ")
}

// PlainPromptPrefix returns a plain "> " for non-TTY output.
func PlainPromptPrefix() string {
	return "> "
}

// AssistantHeader returns a styled header for assistant responses.
func AssistantHeader() string {
	ts := timestampStyle.Render(time.Now().Format("15:04"))
	return fmt.Sprintf("\n  %s  %s", Cyan("assistant"), ts)
}

// AssistantResponse formats and renders an assistant's text response.
// Uses markdown rendering if the terminal supports it.
func AssistantResponse(text string) string {
	if !IsTTY(os.Stdout) {
		return text + "\n"
	}

	rendered := RenderMarkdown(text)
	// Indent the rendered markdown slightly for visual hierarchy
	lines := strings.Split(rendered, "\n")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()}

// StreamingPrefix returns the prefix shown before streaming starts.
func StreamingPrefix() string {
	return thinkingStyle.Render("  ...")
}

// --- Tool Call Formatting ---

// FormatToolCallRich formats a tool call with better visual design.
func FormatToolCallRich(toolName, keyParam, output string, maxOutputLines int) string {
	if !IsTTY(os.Stdout) {
		return FormatToolCall(toolName, keyParam, output, maxOutputLines)
	}

	if maxOutputLines <= 0 {
		maxOutputLines = 5
	}

	var b strings.Builder

	// Tool header with icon
	icon := toolIcon(toolName)
	header := fmt.Sprintf("  %s %s", toolHeaderStyle.Render(icon), toolNameStyle.Render(toolName))
	if keyParam != "" {
		header += " " + sessionInfoStyle.Render(keyParam)
	}
	b.WriteString(header)
	b.WriteByte('\n')

	// Output preview
	output = strings.TrimRight(output, "\n\r ")
	if output != "" {
		lines := strings.Split(output, "\n")
		truncated := false
		if len(lines) > maxOutputLines {
			lines = lines[:maxOutputLines]
			truncated = true
		}
		for _, line := range lines {
			if len(line) > 100 {
				line = line[:97] + "..."
			}
			b.WriteString(toolOutputStyle.Render(line))
			b.WriteByte('\n')
		}
		if truncated {
			b.WriteString(toolOutputStyle.Render("..."))
			b.WriteByte('\n')
		}
	}

	return b.String()}

// toolIcon returns a contextual icon for the tool type.
func toolIcon(toolName string) string {
	switch toolName {
	case "Bash":
		return "$"
	case "Read":
		return ">"
	case "Write":
		return "<"
	case "Edit":
		return "~"
	case "Glob":
		return "*"
	case "Grep":
		return "/"
	default:
		return "-"
	}
}

// --- Step Formatting (for watch/replay) ---

// FormatStepRich formats a replay/watch step with improved visuals.
// stepType: "user", "text", "thinking", "tool", "skill", "error", "recovery", "compaction"
func FormatStepRich(stepType, content string, meta StepMeta) string {
	if !IsTTY(os.Stdout) {
		return formatStepPlain(stepType, content, meta)
	}

	switch stepType {
	case "user":
		return formatUserStep(content, meta)
	case "text":
		return formatAssistantStep(content, meta)
	case "thinking":
		return formatThinkingStep(content, meta)
	case "tool":
		return formatToolStep(content, meta)
	case "skill":
		return formatSkillStep(content, meta)
	case "error":
		return formatErrorStep(content)
	case "recovery":
		return fmt.Sprintf("  %s %s\n", Yellow("↻ recover"), sessionInfoStyle.Render(truncateStr(content, 100)))
	case "compaction":
		return fmt.Sprintf("  %s %s\n", sessionInfoStyle.Render("⊜ compact"), sessionInfoStyle.Render(content))
	default:
		return fmt.Sprintf("  %s\n", content)
	}
}

// StepMeta contains optional metadata for step formatting.
type StepMeta struct {
	ToolName   string
	Path       string
	Command    string
	Skill      string
	Added      int
	Removed    int
	Lines      int
	ExitCode   int
	TimedOut   bool
	DurationMS int64
	Full       bool // show full content without truncation
}

func formatUserStep(content string, meta StepMeta) string {
	if meta.Full {
		return formatMultiLineRich("user", content, promptMarkerStyle)
	}
	text := truncateStr(content, 120)
	return fmt.Sprintf("  %s %s\n", promptMarkerStyle.Render("▸ user"), text)
}

func formatAssistantStep(content string, meta StepMeta) string {
	if meta.Full {
		rendered := RenderMarkdown(content)
		lines := strings.Split(rendered, "\n")
		var b strings.Builder
		b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("● assistant")))
		for _, line := range lines {
			b.WriteString("    ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		return b.String()	}
	text := truncateStr(content, 120)
	return fmt.Sprintf("  %s %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("● assistant"), text)
}

func formatThinkingStep(content string, meta StepMeta) string {
	if meta.Full {
		return formatMultiLineRich("think", content, thinkingStyle)
	}
	text := truncateStr(content, 100)
	return fmt.Sprintf("  %s %s\n", thinkingStyle.Render("◦ think"), sessionInfoStyle.Render(text))
}

func formatToolStep(content string, meta StepMeta) string {
	icon := toolIcon(meta.ToolName)
	name := meta.ToolName
	if name == "" {
		name = "tool"
	}

	detail := ""
	switch name {
	case "Bash":
		cmd := meta.Command
		if len(cmd) > 80 {
			cmd = cmd[:77] + "..."
		}
		detail = cmd
		switch {
		case meta.TimedOut:
			detail += sessionInfoStyle.Render(" timeout")
		case meta.ExitCode != 0:
			detail += errorBlockStyle.Render(fmt.Sprintf(" exit %d", meta.ExitCode))
		case meta.DurationMS > 0:
			detail += sessionInfoStyle.Render(fmt.Sprintf(" %dms", meta.DurationMS))
		}
	case "Read":
		detail = shortPathUI(meta.Path)
	case "Write":
		detail = shortPathUI(meta.Path)
		if meta.Lines > 0 {
			detail += sessionInfoStyle.Render(fmt.Sprintf(" %d lines", meta.Lines))
		}
	case "Edit":
		detail = shortPathUI(meta.Path)
		if meta.Added > 0 || meta.Removed > 0 {
			detail += sessionInfoStyle.Render(fmt.Sprintf(" +%d -%d", meta.Added, meta.Removed))
		}
	case "Glob", "Grep":
		detail = content
	default:
		detail = name
		if meta.Path != "" {
			detail += " " + shortPathUI(meta.Path)
		}
	}

	return fmt.Sprintf("  %s %s %s\n", toolHeaderStyle.Render(icon), toolNameStyle.Render(name), detail)
}

func formatSkillStep(content string, meta StepMeta) string {
	skill := meta.Skill
	if skill == "" {
		skill = content
	}
	return fmt.Sprintf("  %s %s\n", Yellow("◈ skill"), Cyan(skill))
}

func formatErrorStep(content string) string {
	text := content
	if len(text) > 100 {
		text = text[:97] + "..."
	}
	return fmt.Sprintf("  %s %s\n", errorBlockStyle.Render("✗ error"), text)
}

func formatMultiLineRich(label, content string, style lipgloss.Style) string {
	lines := strings.Split(content, "\n")
	var b strings.Builder
	prefix := style.Render("● " + label)
	indent := strings.Repeat(" ", len(label)+4+2) // "  " + icon + " " + label + " "
	for i, line := range lines {
		if i == 0 {
			b.WriteString(fmt.Sprintf("  %s %s\n", prefix, line))
		} else {
			b.WriteString(indent)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()}

func formatStepPlain(stepType, content string, meta StepMeta) string {
	// Plain fallback for non-TTY
	label := fmt.Sprintf("[%-10s]", stepType)
	text := truncateStr(content, 120)
	return fmt.Sprintf("  %s %s\n", label, text)
}

// --- Spinner ---

// SpinnerFrames returns the frames for a minimal spinner animation.
func SpinnerFrames() []string {
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
}

// ThinkingIndicator returns a styled "thinking..." indicator with spinner frame.
func ThinkingIndicator(frame int) string {
	frames := SpinnerFrames()
	f := frames[frame%len(frames)]
	return thinkingStyle.Render(fmt.Sprintf("  %s thinking...", f))
}

// --- Session Summary ---

// FormatSessionSummary returns a styled session completion summary.
func FormatSessionSummary(model string, tokens string, resumeCount, recoveryCount, compactionCount int, lastError, lastRecovery string, failed bool) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(TurnSeparator())
	b.WriteString("\n\n")

	if model != "" {
		b.WriteString(fmt.Sprintf("  %s %s\n", Bold("model:"), sessionInfoStyle.Render(model)))
	}
	if tokens != "" {
		b.WriteString(fmt.Sprintf("  %s %s\n", Bold("tokens:"), sessionInfoStyle.Render(tokens)))
	}
	if resumeCount > 0 {
		b.WriteString(fmt.Sprintf("  %s %d\n", Bold("resumes:"), resumeCount))
	}
	if recoveryCount > 0 {
		b.WriteString(fmt.Sprintf("  %s %d\n", Bold("recoveries:"), recoveryCount))
	}
	if compactionCount > 0 {
		b.WriteString(fmt.Sprintf("  %s %d\n", Bold("compactions:"), compactionCount))
	}
	if lastError != "" && failed {
		b.WriteString(fmt.Sprintf("  %s %s\n", Bold("last error:"), sessionInfoStyle.Render(lastError)))
	}
	if lastRecovery != "" {
		b.WriteString(fmt.Sprintf("  %s %s\n", Bold("last recovery:"), sessionInfoStyle.Render(lastRecovery)))
	}

	return b.String()}

// --- Helpers ---

func shortSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func shortPathUI(p string) string {
	parts := strings.Split(strings.ReplaceAll(p, "\\", "/"), "/")
	if len(parts) <= 3 {
		return p
	}
	return strings.Join(parts[len(parts)-3:], "/")
}

func truncateStr(s string, max int) string {
	// Replace newlines for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
