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
			return printOutput(jsonFlag, sessionID, status, data)
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
					return printOutput(jsonFlag, sessionID, status, partialData)
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

func printOutput(jsonFlag bool, sessionID, status string, data []byte) error {
	if jsonFlag {
		out, _ := json.Marshal(map[string]string{
			"session_id": sessionID,
			"status":     status,
			"output":     string(data),
		})
		fmt.Println(string(out))
		return nil
	}
	fmt.Println(string(data))
	return nil
}
