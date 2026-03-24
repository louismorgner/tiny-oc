package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	skillCmd.AddCommand(skillAddCmd)
}

var skillAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a skill from a public Git repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		url := args[0]
		if !skill.IsURL(url) {
			return fmt.Errorf("expected a URL (https://...), got: %s", url)
		}

		ui.Info("Validating skill at %s...", ui.Dim(url))

		// Clone to temp for validation
		tmpDir, err := os.MkdirTemp("", "toc-skill-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		gitCmd := exec.Command("git", "clone", "--depth", "1", url, tmpDir)
		gitCmd.Stdout = io.Discard
		gitCmd.Stderr = io.Discard
		if err := gitCmd.Run(); err != nil {
			return fmt.Errorf("failed to clone repository — is the URL correct and public?")
		}

		// Find and validate SKILL.md
		skillDir, err := skill.FindSkillMDInDir(tmpDir)
		if err != nil {
			return err
		}

		meta, err := skill.ValidateSkillDir(skillDir)
		if err != nil {
			return err
		}

		// Check for conflicts
		if skill.Exists(meta.Name) {
			return fmt.Errorf("a local skill named '%s' already exists — remove it first or use a different name", meta.Name)
		}

		// Store reference
		if err := skill.AddRef(skill.SkillRef{Name: meta.Name, URL: url}); err != nil {
			return err
		}

		_ = audit.Log("skill.add", map[string]interface{}{
			"skill": meta.Name,
			"url":   url,
		})

		fmt.Println()
		ui.Success("Added skill %s from %s", ui.Bold(meta.Name), ui.Dim(url))
		ui.Info("This skill will be resolved fresh from the URL each time an agent session is spawned.")
		fmt.Println()
		return nil
	},
}
