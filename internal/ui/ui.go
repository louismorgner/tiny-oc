package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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

// Prompt asks for text input with an optional default value.
func Prompt(label, defaultVal string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	if defaultVal != "" {
		fmt.Printf("  %s %s: ", Green("▸"), Bold(label)+" "+Dim("["+defaultVal+"]"))
	} else {
		fmt.Printf("  %s %s: ", Green("▸"), Bold(label))
	}
	val, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
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

	if err := sel.Run(); err != nil {
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
