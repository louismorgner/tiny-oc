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

		s, err := session.FindByIDInWorkspace(ctx.Workspace, sessionID)
		if err != nil {
			return err
		}

		if s.ParentSessionID != ctx.SessionID {
			return fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID)
		}

		outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
		data, err := os.ReadFile(outputPath)
		if err != nil {
			if os.IsNotExist(err) {
				status := s.ResolvedStatus()
				if status == "active" {
					jsonFlag, _ := cmd.Flags().GetBool("json")
					if jsonFlag {
						out, _ := json.Marshal(map[string]string{"status": "running", "output": ""})
						fmt.Println(string(out))
						return nil
					}
					ui.Info("Sub-agent is still running. Output will be available when it completes.")
					return nil
				}
				return fmt.Errorf("no output found for session '%s'", sessionID)
			}
			return err
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			out, _ := json.Marshal(map[string]string{
				"session_id": sessionID,
				"status":     s.ResolvedStatus(),
				"output":     string(data),
			})
			fmt.Println(string(out))
			return nil
		}

		fmt.Println(string(data))
		return nil
	},
}
