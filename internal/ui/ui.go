package ui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/fatih/color"
)

var (
	Bold    = color.New(color.Bold).SprintFunc()
	Green   = color.New(color.FgGreen).SprintFunc()
	Cyan    = color.New(color.FgCyan).SprintFunc()
	Yellow  = color.New(color.FgYellow).SprintFunc()
	Dim     = color.New(color.Faint).SprintFunc()
	Red     = color.New(color.FgRed).SprintFunc()
	BoldCyan = color.New(color.Bold, color.FgCyan).SprintFunc()
)

// WordDeleteFilter remaps ctrl+backspace to ctrl+w so that the bubbles
// textinput recognises it as word-deletion. BubbleTea v2 enables the Kitty
// keyboard protocol, which sends a distinct ctrl+backspace event, but the
// textinput default keymap only binds "alt+backspace" and "ctrl+w".
func WordDeleteFilter(_ tea.Model, msg tea.Msg) tea.Msg {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		if k.Code == tea.KeyBackspace && k.Mod == tea.ModCtrl {
			return tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}
		}
	}
	return msg
}

// FormOptions returns the default tea.ProgramOption set that should be passed
// to any huh form via WithProgramOptions. It includes the word-delete filter
// and the stderr output that huh uses by default.
func FormOptions() []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithOutput(os.Stderr),
		tea.WithFilter(WordDeleteFilter),
	}
}

// runField wraps a single huh field in a form with our standard program options.
func runField(field huh.Field) error {
	return huh.NewForm(huh.NewGroup(field)).
		WithShowHelp(false).
		WithProgramOptions(FormOptions()...).
		Run()
}

// Prompt asks for text input with an optional default value.
func Prompt(label, defaultVal string) (string, error) {
	var val string
	input := huh.NewInput().
		Title(label).
		Value(&val)
	if defaultVal != "" {
		input = input.Description("default: " + defaultVal)
	}
	if err := runField(input); err != nil {
		return "", err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

// Confirm asks a yes/no question. Returns true if the user confirms.
func Confirm(label string, defaultVal bool) (bool, error) {
	var result bool = defaultVal
	c := huh.NewConfirm().
		Title(label).
		Value(&result)
	if err := runField(c); err != nil {
		return false, err
	}
	return result, nil
}

type SelectOption struct {
	Label string
	Value string
}

// Select shows an interactive arrow-key select menu.
func Select(label string, options []SelectOption, defaultIdx int) (string, error) {
	var result string

	huhOpts := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOpts[i] = huh.NewOption(opt.Label, opt.Value)
	}

	sel := huh.NewSelect[string]().
		Title(label).
		Options(huhOpts...).
		Value(&result)

	if defaultIdx >= 0 && defaultIdx < len(options) {
		result = options[defaultIdx].Value
	}

	if err := runField(sel); err != nil {
		return "", err
	}
	return result, nil
}

// Success prints a green success message.
func Success(format string, args ...interface{}) {
	fmt.Printf("  %s %s\n", Green("✓"), fmt.Sprintf(format, args...))
}

// Info prints an info message.
func Info(format string, args ...interface{}) {
	fmt.Printf("  %s %s\n", Cyan("→"), fmt.Sprintf(format, args...))
}

// Warn prints a yellow warning message.
func Warn(format string, args ...interface{}) {
	fmt.Printf("  %s %s\n", Yellow("⚠"), fmt.Sprintf(format, args...))
}

// Error prints a red error message.
func Error(format string, args ...interface{}) {
	fmt.Printf("  %s %s\n", Red("✗"), fmt.Sprintf(format, args...))
}

// Header prints a bold section header.
func Header(text string) {
	fmt.Printf("\n  %s\n\n", BoldCyan(text))
}

// Command prints a copyable command string.
func Command(cmd string) {
	fmt.Printf("  %s %s\n", Dim("$"), Bold(cmd))
}
