package cmd

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/ui"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func init() {
	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update toc to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println()

		current := version

		if current == "dev" {
			ui.Warn("Running a dev build — skipping update check")
			fmt.Println()
			return nil
		}

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
		baseURL := fmt.Sprintf("https://github.com/louismorgner/tiny-oc/releases/download/v%s", latest)

		ui.Info("Downloading %s...", archive)

		// Download to temp dir
		tmpDir, err := os.MkdirTemp("", "toc-update-*")
		if err != nil {
			return fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		archivePath := filepath.Join(tmpDir, archive)
		if err := downloadFile(baseURL+"/"+archive, archivePath); err != nil {
			return fmt.Errorf("failed to download update: %w", err)
		}

		// Download and verify checksum
		checksumURL := baseURL + "/checksums.txt"
		checksumPath := filepath.Join(tmpDir, "checksums.txt")
		if err := downloadFile(checksumURL, checksumPath); err != nil {
			return fmt.Errorf("failed to download checksums: %w", err)
		}

		if err := verifyChecksum(archivePath, checksumPath, archive); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		ui.Success("Checksum verified")

		// Extract
		newBinary := filepath.Join(tmpDir, "toc")
		if err := extractTarGz(archivePath, "toc", newBinary); err != nil {
			return fmt.Errorf("failed to extract archive: %w", err)
		}

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
	resp, err := httpClient.Get("https://api.github.com/repos/louismorgner/tiny-oc/releases/latest")
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
	resp, err := httpClient.Get(url)
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

func verifyChecksum(archivePath, checksumPath, archiveName string) error {
	// Compute SHA256 of the downloaded archive
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))

	// Parse checksums.txt for the expected hash
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == archiveName {
			if parts[0] != actual {
				return fmt.Errorf("expected %s, got %s", parts[0], actual)
			}
			return nil
		}
	}

	return fmt.Errorf("no checksum found for %s", archiveName)
}

func extractTarGz(archivePath, targetName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name == targetName {
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, tr)
			return err
		}
	}
	return fmt.Errorf("%s not found in archive", targetName)
}

func replaceBinary(src, dst string) error {
	// Try atomic rename first (works on same filesystem)
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	// Fall back to copy-to-tmpfile-then-rename for cross-device moves
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	tmpDst := dst + ".tmp"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(tmpDst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmpDst)
		return err
	}
	out.Close()
	return os.Rename(tmpDst, dst)
}
