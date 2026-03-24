package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/registry"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

var skillAddRegistry bool

func init() {
	skillAddCmd.Flags().BoolVar(&skillAddRegistry, "registry", false, "Install from the toc skill registry")
	skillCmd.AddCommand(skillAddCmd)
}

var skillAddCmd = &cobra.Command{
	Use:   "add <url-or-name>",
	Short: "Add a skill from a URL or the registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		arg := args[0]

		if skillAddRegistry {
			return addFromRegistry(arg)
		}

		if skill.IsURL(arg) {
			return addFromURL(arg)
		}

		return fmt.Errorf("expected a URL (https://...) or use --registry flag to install by name\n\nExamples:\n  toc skill add https://github.com/user/skill-repo\n  toc skill add --registry code-review")
	},
}

func addFromRegistry(name string) error {
	if err := skill.ValidateName(name); err != nil {
		return err
	}

	if skill.Exists(name) {
		return fmt.Errorf("skill '%s' already exists locally — remove it first with: toc skill remove %s", name, name)
	}

	ui.Info("Fetching registry index...")
	index, err := registry.FetchIndex()
	if err != nil {
		return err
	}

	entry, found := registry.FindSkill(index, name)
	if !found {
		return fmt.Errorf("skill '%s' not found in registry — run 'toc registry search' to see available skills", name)
	}

	ui.Info("Installing %s — %s", ui.Bold(entry.Name), ui.Dim(entry.Description))

	meta, err := registry.InstallSkill(name)
	if err != nil {
		return err
	}

	_ = audit.Log("skill.add", map[string]interface{}{
		"skill":  meta.Name,
		"source": "registry",
	})

	fmt.Println()
	ui.Success("Installed skill %s from registry", ui.Bold(meta.Name))
	ui.Info("Assign it to an agent with: %s", ui.Bold(fmt.Sprintf("toc agent skills <agent-name>")))
	fmt.Println()
	return nil
}

func addFromURL(url string) error {
	ui.Info("Validating skill at %s...", ui.Dim(url))

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

	skillDir, err := skill.FindSkillMDInDir(tmpDir)
	if err != nil {
		return err
	}

	meta, err := skill.ValidateSkillDir(skillDir)
	if err != nil {
		return err
	}

	if skill.Exists(meta.Name) {
		return fmt.Errorf("a local skill named '%s' already exists — remove it first or use a different name", meta.Name)
	}

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
}
