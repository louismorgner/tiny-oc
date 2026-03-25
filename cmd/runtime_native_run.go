package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/runtime"
)

var (
	nativeRunMode      string
	nativeRunDir       string
	nativeRunSessionID string
	nativeRunAgent     string
	nativeRunWorkspace string
	nativeRunModel     string
	nativeRunPrompt    string
	nativeRunResume    bool
)

func init() {
	nativeRunCmd.Flags().StringVar(&nativeRunMode, "mode", "interactive", "Execution mode")
	nativeRunCmd.Flags().StringVar(&nativeRunDir, "dir", "", "Session working directory")
	nativeRunCmd.Flags().StringVar(&nativeRunSessionID, "session-id", "", "Session ID")
	nativeRunCmd.Flags().StringVar(&nativeRunAgent, "agent", "", "Agent name")
	nativeRunCmd.Flags().StringVar(&nativeRunWorkspace, "workspace", "", "Workspace path")
	nativeRunCmd.Flags().StringVar(&nativeRunModel, "model", "", "Model identifier")
	nativeRunCmd.Flags().StringVar(&nativeRunPrompt, "prompt-file", "", "Optional prompt file")
	nativeRunCmd.Flags().BoolVar(&nativeRunResume, "resume", false, "Resume an existing session")
	rootCmd.AddCommand(nativeRunCmd)
}

var nativeRunCmd = &cobra.Command{
	Use:    "__native-run",
	Short:  "Internal toc-native runtime entrypoint",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if nativeRunDir == "" || nativeRunSessionID == "" || nativeRunAgent == "" || nativeRunWorkspace == "" {
			return fmt.Errorf("missing required native runtime flags")
		}

		prompt, err := loadNativePrompt(nativeRunPrompt)
		if err != nil {
			return err
		}

		return runtime.RunNativeSession(runtime.NativeRunOptions{
			Mode:      nativeRunMode,
			Dir:       nativeRunDir,
			SessionID: nativeRunSessionID,
			Agent:     nativeRunAgent,
			Workspace: nativeRunWorkspace,
			Model:     nativeRunModel,
			Prompt:    prompt,
			Resume:    nativeRunResume,
		}, os.Stdin, os.Stdout)
	},
}

func loadNativePrompt(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
