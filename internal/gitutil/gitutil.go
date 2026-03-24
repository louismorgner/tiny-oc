package gitutil

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// SafeClone clones a git repository with hooks disabled to prevent
// arbitrary code execution from malicious repositories.
func SafeClone(url, destDir string, extraArgs ...string) error {
	if err := ValidateURL(url); err != nil {
		return err
	}

	args := []string{
		"-c", "core.hooksPath=/dev/null",
		"clone", "--depth", "1",
	}
	args = append(args, extraArgs...)
	args = append(args, url, destDir)

	cmd := exec.Command("git", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone %s: %w", url, err)
	}
	return nil
}

// ValidateURL checks that a URL uses HTTPS. Plain HTTP is rejected
// to prevent man-in-the-middle attacks that could inject malicious content.
func ValidateURL(url string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("only HTTPS URLs are allowed (got %q)", url)
	}
	return nil
}
