package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/gitutil"
	"github.com/tiny-oc/toc/internal/registry"
	"github.com/tiny-oc/toc/internal/skill"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	skillCmd.AddCommand(skillAddCmd)
}

var skillAddCmd = &cobra.Command{
	Use:   "add <url-or-name>",
	Short: "Add a skill from a Git URL or the registry",
	Long:  "Add a skill by Git URL or registry name. If the argument is not a URL, the registry is searched automatically.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		arg := args[0]

		// If it's not a URL, try the registry.
		if !skill.IsURL(arg) {
			return addFromRegistry(arg)
		}

		return addFromURL(arg)
	},
}

func addFromRegistry(name string) error {
	ui.Info("Fetching registry...")
	index, err := registry.FetchIndex()
	if err != nil {
		return err
	}

	entry, found := registry.FindSkill(index, name)
	if !found {
		// Check if it exists as a different type to give a better error.
		if other, ok := registry.Find(index, name); ok {
			return fmt.Errorf("'%s' is an %s, not a skill — use 'toc agent add %s' instead", name, other.Type, name)
		}
		return fmt.Errorf("'%s' is not a URL and was not found in the registry\n\nExamples:\n  toc skill add https://github.com/user/skill-repo\n  toc registry search", name)
	}

	ui.Info("Installing %s — %s", ui.Bold(entry.Name), ui.Dim(entry.Description))

	if err := registry.Install(entry); err != nil {
		return err
	}

	auditLog("skill.add", map[string]interface{}{
		"skill":  entry.Name,
		"source": "registry",
	})

	fmt.Println()
	ui.Success("Installed skill %s from registry", ui.Bold(entry.Name))
	ui.Info("Assign to an agent with: %s", ui.Bold("toc agent skills <agent-name>"))
	fmt.Println()
	return nil
}

func addFromURL(url string) error {
	if err := gitutil.ValidateURL(url); err != nil {
		return fmt.Errorf("expected an HTTPS URL (https://...)\n\nTo install from the registry: toc skill add <name>")
	}

	ui.Info("Validating skill at %s...", ui.Dim(url))

	tmpDir, err := os.MkdirTemp("", "toc-skill-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := gitutil.SafeClone(url, tmpDir); err != nil {
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

	auditLog("skill.add", map[string]interface{}{
		"skill": meta.Name,
		"url":   url,
	})

	fmt.Println()
	ui.Success("Added skill %s from %s", ui.Bold(meta.Name), ui.Dim(url))
	ui.Info("This skill will be resolved fresh from the URL each time an agent session is spawned.")
	fmt.Println()
	return nil
}
