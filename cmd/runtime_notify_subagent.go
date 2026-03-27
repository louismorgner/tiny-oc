package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
)

var (
	notifyWorkspace     string
	notifyParentSession string
	notifySessionID     string
	notifyAgent         string
	notifyPromptFile    string
	notifyOutputFile    string
	notifyExitCodeFile  string
)

func init() {
	notifySubAgentCmd.Flags().StringVar(&notifyWorkspace, "workspace", "", "Workspace path")
	notifySubAgentCmd.Flags().StringVar(&notifyParentSession, "parent-session-id", "", "Parent session ID")
	notifySubAgentCmd.Flags().StringVar(&notifySessionID, "session-id", "", "Sub-agent session ID")
	notifySubAgentCmd.Flags().StringVar(&notifyAgent, "agent", "", "Sub-agent agent name")
	notifySubAgentCmd.Flags().StringVar(&notifyPromptFile, "prompt-file", "", "Path to the sub-agent prompt file")
	notifySubAgentCmd.Flags().StringVar(&notifyOutputFile, "output-file", "", "Path to the final sub-agent output file")
	notifySubAgentCmd.Flags().StringVar(&notifyExitCodeFile, "exit-code-file", "", "Path to the sub-agent exit code file")
	rootCmd.AddCommand(notifySubAgentCmd)
}

var notifySubAgentCmd = &cobra.Command{
	Use:          "__notify-subagent-complete",
	Short:        "Internal sub-agent completion notifier",
	Hidden:       true,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if notifyWorkspace == "" || notifyParentSession == "" || notifySessionID == "" || notifyAgent == "" {
			return fmt.Errorf("missing required notification flags")
		}

		prompt, err := readOptionalFile(notifyPromptFile)
		if err != nil {
			return err
		}
		output, err := readOptionalFile(notifyOutputFile)
		if err != nil {
			return err
		}
		exitCode, err := readExitCodeFile(notifyExitCodeFile)
		if err != nil {
			return err
		}

		status := session.StatusCompletedOK
		if exitCode != 0 {
			status = session.StatusCompletedError
		}

		_, err = runtime.WriteSubAgentCompletionNotification(notifyWorkspace, notifyParentSession, runtime.SessionNotification{
			SessionID: notifySessionID,
			Agent:     notifyAgent,
			Status:    status,
			ExitCode:  exitCode,
			Prompt:    prompt,
			Output:    output,
		})
		return err
	},
}

func readOptionalFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func readExitCodeFile(path string) (int, error) {
	if strings.TrimSpace(path) == "" {
		return 0, nil
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return code, nil
}
