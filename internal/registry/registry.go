package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tiny-oc/toc/internal/skill"
	"gopkg.in/yaml.v3"
)

const (
	RepoURL  = "https://github.com/tiny-oc/toc.git"
	IndexURL = "https://raw.githubusercontent.com/tiny-oc/toc/main/registry/skills/index.yaml"
)

// SkillEntry represents a skill in the registry index.
type SkillEntry struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// SkillIndex is the top-level registry index.
type SkillIndex struct {
	Skills []SkillEntry `yaml:"skills" json:"skills"`
}

// FetchIndex downloads and parses the skills registry index from GitHub.
func FetchIndex() (*SkillIndex, error) {
	resp, err := http.Get(IndexURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch registry index: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry index: %w", err)
	}

	var index SkillIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse registry index: %w", err)
	}

	return &index, nil
}

// Search filters the index by a query string, matching against name, description, and tags.
func Search(index *SkillIndex, query string) []SkillEntry {
	if query == "" {
		return index.Skills
	}
	q := strings.ToLower(query)
	var results []SkillEntry
	for _, s := range index.Skills {
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Description), q) ||
			matchTags(s.Tags, q) {
			results = append(results, s)
		}
	}
	return results
}

func matchTags(tags []string, query string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}
	return false
}

// FindSkill looks up a skill by exact name in the index.
func FindSkill(index *SkillIndex, name string) (*SkillEntry, bool) {
	for _, s := range index.Skills {
		if s.Name == name {
			return &s, true
		}
	}
	return nil, false
}

// InstallSkill clones the toc repo and copies the skill into the local workspace (.toc/skills/).
func InstallSkill(name string) (*skill.SkillMeta, error) {
	return InstallSkillTo(name, skill.Dir(name))
}

// InstallSkillTo clones the toc repo and copies the skill into the given target directory.
// Used by both `toc skill add --registry` (installs to .toc/skills/) and spawn-time
// resolution (installs to session .claude/skills/).
func InstallSkillTo(name, destDir string) (*skill.SkillMeta, error) {
	tmpDir, err := os.MkdirTemp("", "toc-registry-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Sparse clone — only fetch the specific skill directory
	gitCmd := exec.Command("git", "clone", "--depth", "1", "--filter=blob:none", "--sparse", RepoURL, tmpDir)
	gitCmd.Stdout = io.Discard
	gitCmd.Stderr = io.Discard
	if err := gitCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to clone registry repository")
	}

	sparseCmd := exec.Command("git", "-C", tmpDir, "sparse-checkout", "set", fmt.Sprintf("registry/skills/%s", name))
	sparseCmd.Stdout = io.Discard
	sparseCmd.Stderr = io.Discard
	if err := sparseCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to configure sparse checkout")
	}

	// Validate the skill
	skillSrc := filepath.Join(tmpDir, "registry", "skills", name)
	if _, err := os.Stat(skillSrc); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill '%s' not found in registry", name)
	}

	meta, err := skill.ValidateSkillDir(skillSrc)
	if err != nil {
		return nil, fmt.Errorf("invalid skill in registry: %w", err)
	}

	if err := copyDir(skillSrc, destDir); err != nil {
		return nil, fmt.Errorf("failed to install skill: %w", err)
	}

	return meta, nil
}

// FormatJSON returns the index as formatted JSON (for --json flag).
func FormatJSON(entries []SkillEntry) (string, error) {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
