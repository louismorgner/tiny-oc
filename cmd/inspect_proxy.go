package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/inspect"
)

var (
	inspectProxyListenAddr string
	inspectProxyUpstream   string
	inspectProxyCapture    string
	inspectProxyReady      string
)

func init() {
	inspectProxyCmd.Flags().StringVar(&inspectProxyListenAddr, "listen-addr", "127.0.0.1:0", "Listen address")
	inspectProxyCmd.Flags().StringVar(&inspectProxyUpstream, "upstream", "", "Upstream base URL")
	inspectProxyCmd.Flags().StringVar(&inspectProxyCapture, "capture-file", "", "Capture JSONL output path")
	inspectProxyCmd.Flags().StringVar(&inspectProxyReady, "ready-file", "", "Path written when the proxy is ready")
	rootCmd.AddCommand(inspectProxyCmd)
}

var inspectProxyCmd = &cobra.Command{
	Use:          inspect.HelperCommandName,
	Short:        "Internal inspector reverse proxy",
	Hidden:       true,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if inspectProxyUpstream == "" || inspectProxyCapture == "" || inspectProxyReady == "" {
			return fmt.Errorf("missing required inspect proxy flags")
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return inspect.RunProxy(ctx, inspectProxyListenAddr, inspectProxyUpstream, inspectProxyCapture, inspectProxyReady)
	},
}
