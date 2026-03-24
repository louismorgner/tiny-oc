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

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/skill"
	"gopkg.in/yaml.v3"
)

const (
	RepoURL        = "https://github.com/louismorgner/tiny-oc.git"
	rawBase        = "https://raw.githubusercontent.com/louismorgner/tiny-oc/main/registry"
	SkillIndexURL  = rawBase + "/skills/index.yaml"
	AgentIndexURL  = rawBase + "/agents/index.yaml"
)

// Entry represents any item in the registry — skill or agent.
type Entry struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Type        string   `yaml:"-" json:"type"` // "skill" or "agent"
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	Skills      []string `yaml:"skills,omitempty" json:"skills,omitempty"`
}

type skillIndex struct {
	Skills []Entry `yaml:"skills"`
}

type agentIndex struct {
	Agents []Entry `yaml:"agents"`
}

// Index holds all registry entries.
type Index struct {
	Entries []Entry
}

// FetchIndex downloads both skill and agent indexes and merges them.
func FetchIndex() (*Index, error) {
	index := &Index{}

	skills, err := fetchYAML[skillIndex](SkillIndexURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch skills index: %w", err)
	}
	for _, s := range skills.Skills {
		s.Type = "skill"
		index.Entries = append(index.Entries, s)
	}

	agents, err := fetchYAML[agentIndex](AgentIndexURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agents index: %w", err)
	}
	for _, a := range agents.Agents {
		a.Type = "agent"
		index.Entries = append(index.Entries, a)
	}

	return index, nil
}

func fetchYAML[T any](url string) (*T, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result T
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Search filters entries by query, matching name, description, tags, and type.
func Search(index *Index, query string) []Entry {
	if query == "" {
		return index.Entries
	}
	q := strings.ToLower(query)
	var results []Entry
	for _, e := range index.Entries {
		if strings.Contains(strings.ToLower(e.Name), q) ||
			strings.Contains(strings.ToLower(e.Description), q) ||
			strings.Contains(e.Type, q) ||
			matchTags(e.Tags, q) {
			results = append(results, e)
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

// Find looks up an entry by exact name.
func Find(index *Index, name string) (*Entry, bool) {
	for _, e := range index.Entries {
		if e.Name == name {
			return &e, true
		}
	}
	return nil, false
}

// FindSkill looks up a skill by exact name (for backward compat with spawn resolution).
func FindSkill(index *Index, name string) (*Entry, bool) {
	for _, e := range index.Entries {
		if e.Name == name && e.Type == "skill" {
			return &e, true
		}
	}
	return nil, false
}

// Install installs a registry entry into the local workspace.
// For skills: copies to .toc/skills/<name>/
// For agents: copies to .toc/agents/<name>/ and installs referenced skills
func Install(entry *Entry) error {
	switch entry.Type {
	case "skill":
		return installSkill(entry.Name)
	case "agent":
		return installAgent(entry)
	default:
		return fmt.Errorf("unknown registry entry type: %s", entry.Type)
	}
}

func installSkill(name string) error {
	if skill.Exists(name) {
		return fmt.Errorf("skill '%s' already exists locally", name)
	}
	_, err := InstallSkillTo(name, skill.Dir(name))
	return err
}

func installAgent(entry *Entry) error {
	if agent.Exists(entry.Name) {
		return fmt.Errorf("agent '%s' already exists locally", entry.Name)
	}

	destDir := agent.Dir(entry.Name)
	if err := cloneRegistryDir("agents/"+entry.Name, destDir); err != nil {
		return err
	}

	// Install referenced skills that aren't already local
	for _, s := range entry.Skills {
		if skill.Exists(s) {
			continue
		}
		if _, err := InstallSkillTo(s, skill.Dir(s)); err != nil {
			return fmt.Errorf("failed to install skill '%s' referenced by agent: %w", s, err)
		}
	}

	return nil
}

// InstallSkillTo clones a skill from the registry into the given directory.
func InstallSkillTo(name, destDir string) (*skill.SkillMeta, error) {
	if err := cloneRegistryDir("skills/"+name, destDir); err != nil {
		return nil, err
	}

	meta, err := skill.ValidateSkillDir(destDir)
	if err != nil {
		os.RemoveAll(destDir)
		return nil, fmt.Errorf("invalid skill in registry: %w", err)
	}

	return meta, nil
}

// cloneRegistryDir does a sparse clone of a specific directory from the registry.
func cloneRegistryDir(registryPath, destDir string) error {
	tmpDir, err := os.MkdirTemp("", "toc-registry-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	gitCmd := exec.Command("git", "clone", "--depth", "1", "--filter=blob:none", "--sparse", RepoURL, tmpDir)
	gitCmd.Stdout = io.Discard
	gitCmd.Stderr = io.Discard
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone registry repository")
	}

	sparseCmd := exec.Command("git", "-C", tmpDir, "sparse-checkout", "set", "registry/"+registryPath)
	sparseCmd.Stdout = io.Discard
	sparseCmd.Stderr = io.Discard
	if err := sparseCmd.Run(); err != nil {
		return fmt.Errorf("failed to configure sparse checkout")
	}

	srcDir := filepath.Join(tmpDir, "registry", registryPath)
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return fmt.Errorf("'%s' not found in registry", registryPath)
	}

	return copyDir(srcDir, destDir)
}

// FormatJSON returns entries as formatted JSON.
func FormatJSON(entries []Entry) (string, error) {
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
