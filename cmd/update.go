package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update toc to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println()

		current := version
		ui.Info("Current version: %s", ui.Bold(current))

		// Fetch latest release from GitHub
		latest, err := fetchLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}

		if current == latest {
			ui.Success("Already up to date!")
			fmt.Println()
			return nil
		}

		ui.Info("Latest version:  %s", ui.Bold(latest))
		fmt.Println()

		// Determine platform
		goos := runtime.GOOS
		goarch := runtime.GOARCH

		archive := fmt.Sprintf("toc_%s_%s_%s.tar.gz", latest, goos, goarch)
		url := fmt.Sprintf("https://github.com/louismorgner/tiny-oc/releases/download/v%s/%s", latest, archive)

		ui.Info("Downloading %s...", archive)

		// Download to temp dir
		tmpDir, err := os.MkdirTemp("", "toc-update-*")
		if err != nil {
			return fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		archivePath := filepath.Join(tmpDir, archive)
		if err := downloadFile(url, archivePath); err != nil {
			return fmt.Errorf("failed to download update: %w", err)
		}

		// Extract
		extractCmd := exec.Command("tar", "-xzf", archivePath, "-C", tmpDir)
		if err := extractCmd.Run(); err != nil {
			return fmt.Errorf("failed to extract archive: %w", err)
		}

		newBinary := filepath.Join(tmpDir, "toc")
		if err := os.Chmod(newBinary, 0755); err != nil {
			return fmt.Errorf("failed to set permissions: %w", err)
		}

		// Find current binary path
		currentBinary, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to find current binary: %w", err)
		}
		currentBinary, err = filepath.EvalSymlinks(currentBinary)
		if err != nil {
			return fmt.Errorf("failed to resolve binary path: %w", err)
		}

		// Replace current binary
		if err := replaceBinary(newBinary, currentBinary); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}

		ui.Success("Updated toc %s → %s", current, ui.Green(latest))
		fmt.Println()
		return nil
	},
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func fetchLatestVersion() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/louismorgner/tiny-oc/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func replaceBinary(src, dst string) error {
	// Atomic-ish replacement: rename new over old.
	// On Unix, we can rename over a running binary.
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0755)
}
