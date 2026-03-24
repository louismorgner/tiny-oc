package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

var (
	auditTail   int
	auditJSON   bool
	auditAction string
)

func init() {
	auditCmd.Flags().IntVar(&auditTail, "tail", 20, "Show last N entries")
	auditCmd.Flags().BoolVar(&auditJSON, "json", false, "Output raw JSON lines")
	auditCmd.Flags().StringVar(&auditAction, "action", "", "Filter by action prefix (e.g. agent, session)")
	rootCmd.AddCommand(auditCmd)
}

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "View the audit log",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		events, err := audit.Read(auditTail, auditAction)
		if err != nil {
			return err
		}

		if len(events) == 0 {
			ui.Info("No audit events found.")
			return nil
		}

		if auditJSON {
			for _, e := range events {
				line, _ := json.Marshal(e)
				fmt.Println(string(line))
			}
			return nil
		}

		fmt.Println()
		for _, e := range events {
			ts, _ := time.Parse("2006-01-02T15:04:05.000Z", e.Timestamp)
			localTime := ts.Local().Format("2006-01-02 15:04")

			details := formatDetails(e.Details)
			fmt.Printf("  %s  %-20s %s  %s\n",
				ui.Dim(localTime),
				actionColor(e.Action),
				ui.Dim(e.Actor),
				ui.Dim(details),
			)
		}
		fmt.Println()

		return nil
	},
}

func formatDetails(details map[string]interface{}) string {
	if len(details) == 0 {
		return ""
	}
	var parts []string
	// Sort keys for consistent output
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := details[k]
		s := fmt.Sprintf("%v", v)
		// Truncate long values like session IDs
		if len(s) > 12 && k == "session_id" {
			s = s[:8]
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return strings.Join(parts, " ")
}

func actionColor(action string) string {
	switch {
	case strings.HasPrefix(action, "workspace"):
		return ui.BoldCyan(action)
	case strings.HasPrefix(action, "agent"):
		return ui.Cyan(action)
	case strings.HasPrefix(action, "session"):
		return ui.Green(action)
	case strings.HasPrefix(action, "context"):
		return ui.Yellow(action)
	default:
		return action
	}
}
