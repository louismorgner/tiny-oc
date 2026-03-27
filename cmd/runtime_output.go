package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeOutputCmd.Flags().Bool("json", false, "Output structured JSON")
	runtimeOutputCmd.Flags().Bool("partial", false, "Read partial output from a running session")
	runtimeOutputCmd.Flags().Bool("meta", false, "Show session metadata before output")
	runtimeCmd.AddCommand(runtimeOutputCmd)
}

var runtimeOutputCmd = &cobra.Command{
	Use:   "output <session-id>",
	Short: "Read the output of a sub-agent session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		sessionID := args[0]
		jsonFlag, _ := cmd.Flags().GetBool("json")
		partialFlag, _ := cmd.Flags().GetBool("partial")
		metaFlag, _ := cmd.Flags().GetBool("meta")

		s, err := session.FindByIDInWorkspace(ctx.Workspace, sessionID)
		if err != nil {
			return err
		}

		if s.ParentSessionID != ctx.SessionID {
			return fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID)
		}

		status := s.ResolvedStatus()

		// Try reading the final output file first
		outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
		data, err := os.ReadFile(outputPath)
		if err == nil {
			return printOutput(jsonFlag, metaFlag, s, status, data)
		}

		if !os.IsNotExist(err) {
			return err
		}

		// Final output doesn't exist — try partial output if requested or still active
		if partialFlag || status == "active" {
			tmpPath := filepath.Join(s.WorkspacePath, "toc-output.txt.tmp")
			partialData, tmpErr := os.ReadFile(tmpPath)
			if tmpErr == nil && len(partialData) > 0 {
				if partialFlag {
					return printOutput(jsonFlag, metaFlag, s, status, partialData)
				}
				// Active but not --partial: hint the user
				if jsonFlag {
					out, _ := json.Marshal(map[string]string{"status": "running", "output": ""})
					fmt.Println(string(out))
					return nil
				}
				ui.Info("Sub-agent is still running. Use --partial to read output so far.")
				return nil
			}

			// No partial output yet
			if jsonFlag {
				out, _ := json.Marshal(map[string]string{"status": "running", "output": ""})
				fmt.Println(string(out))
				return nil
			}
			ui.Info("Sub-agent is still running. No output yet.")
			return nil
		}

		return fmt.Errorf("no output found for session '%s'", sessionID)
	},
}

func printOutput(jsonFlag, metaFlag bool, s *session.Session, status string, data []byte) error {
	summary, err := loadRuntimeStateSummary(s)
	if err != nil {
		return err
	}

	if jsonFlag {
		out, _ := json.Marshal(map[string]interface{}{
			"session_id":       s.ID,
			"status":           status,
			"runtime":          s.RuntimeName(),
			"runtime_state":    summaryField(summary, func(v *runtimeStateSummary) string { return v.Status }),
			"resume_count":     summaryIntField(summary, func(v *runtimeStateSummary) int { return v.ResumeCount }),
			"recovery_count":   summaryIntField(summary, func(v *runtimeStateSummary) int { return v.RecoveryCount }),
			"compaction_count": summaryIntField(summary, func(v *runtimeStateSummary) int { return v.CompactionCount }),
			"last_error":       summaryField(summary, func(v *runtimeStateSummary) string { return v.LastError }),
			"last_recovery":    summaryField(summary, func(v *runtimeStateSummary) string { return v.LastRecovery }),
			"todos":            summaryTodos(summary),
			"token_total":      summaryTokenField(summary),
			"output":           string(data),
		})
		fmt.Println(string(out))
		return nil
	}
	if metaFlag {
		fmt.Println()
		fmt.Printf("  %s %s\n", ui.Bold("Session:"), ui.Dim(s.ID))
		fmt.Printf("  %s %s\n", ui.Bold("Status:"), ui.Dim(status))
		fmt.Printf("  %s %s\n", ui.Bold("Runtime:"), ui.Dim(s.RuntimeName()))
		if summary != nil {
			if summary.Model != "" {
				fmt.Printf("  %s %s\n", ui.Bold("Model:"), ui.Dim(summary.Model))
			}
			if summary.ResumeCount > 0 {
				fmt.Printf("  %s %d\n", ui.Bold("Resumes:"), summary.ResumeCount)
			}
			if summary.RecoveryCount > 0 {
				fmt.Printf("  %s %d\n", ui.Bold("Recoveries:"), summary.RecoveryCount)
			}
			if summary.CompactionCount > 0 {
				fmt.Printf("  %s %d\n", ui.Bold("Compactions:"), summary.CompactionCount)
			}
			if total := summary.Tokens.FormatTotal(); total != "" {
				fmt.Printf("  %s %s\n", ui.Bold("Tokens:"), ui.Dim(total))
			}
			if summary.LastError != "" {
				fmt.Printf("  %s %s\n", ui.Bold("Last error:"), ui.Dim(summary.LastError))
			}
			if summary.LastRecovery != "" {
				fmt.Printf("  %s %s\n", ui.Bold("Last recovery:"), ui.Dim(summary.LastRecovery))
			}
			if todoLines := formatTodoLines(summary.Todos); len(todoLines) > 0 {
				fmt.Printf("  %s\n", ui.Bold("Todos:"))
				for _, line := range todoLines {
					fmt.Printf("    %s %s\n", ui.Dim("•"), ui.Dim(line))
				}
			}
		}
		fmt.Println()
	}
	fmt.Println(string(data))
	return nil
}

func summaryField(summary *runtimeStateSummary, get func(*runtimeStateSummary) string) string {
	if summary == nil {
		return ""
	}
	return get(summary)
}

func summaryIntField(summary *runtimeStateSummary, get func(*runtimeStateSummary) int) int {
	if summary == nil {
		return 0
	}
	return get(summary)
}

func summaryTokenField(summary *runtimeStateSummary) int64 {
	if summary == nil {
		return 0
	}
	return summary.Tokens.Total()
}

func summaryTodos(summary *runtimeStateSummary) []runtime.TodoItem {
	if summary == nil {
		return nil
	}
	return append([]runtime.TodoItem(nil), summary.Todos...)
}
