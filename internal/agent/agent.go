package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/tiny-oc/toc/internal/config"
	"gopkg.in/yaml.v3"
)

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type AgentConfig struct {
	Runtime     string   `yaml:"runtime"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Model       string   `yaml:"model"`
	Context     []string `yaml:"context,omitempty"`
	Skills      []string `yaml:"skills,omitempty"`
	SubAgents   []string `yaml:"sub-agents,omitempty"`
}

// CanSpawn checks if this agent is allowed to spawn the given target agent.
func (cfg *AgentConfig) CanSpawn(target string) bool {
	for _, entry := range cfg.SubAgents {
		if entry == "*" || entry == target {
			return true
		}
	}
	return false
}

// CanSpawnAny returns true if the agent has any sub-agent permissions.
func (cfg *AgentConfig) CanSpawnAny() bool {
	return len(cfg.SubAgents) > 0
}

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("agent name must be lowercase alphanumeric with hyphens (e.g. 'pr-reviewer')")
	}
	return nil
}

var validRuntimes = map[string]bool{"claude-code": true}
var validModels = map[string]bool{"sonnet": true, "opus": true, "haiku": true}

// Validate checks the agent config for errors. Returns a list of problems found.
func (cfg *AgentConfig) Validate() []string {
	var problems []string
	if cfg.Name == "" {
		problems = append(problems, "missing name")
	} else if err := ValidateName(cfg.Name); err != nil {
		problems = append(problems, err.Error())
	}
	if cfg.Runtime == "" {
		problems = append(problems, "missing runtime")
	} else if !validRuntimes[cfg.Runtime] {
		problems = append(problems, fmt.Sprintf("unknown runtime: %s", cfg.Runtime))
	}
	if cfg.Model == "" {
		problems = append(problems, "missing model")
	} else if !validModels[cfg.Model] {
		problems = append(problems, fmt.Sprintf("unknown model: %s (expected sonnet, opus, or haiku)", cfg.Model))
	}
	return problems
}

func Dir(name string) string {
	return filepath.Join(config.AgentsDir(), name)
}

func Exists(name string) bool {
	_, err := os.Stat(Dir(name))
	return err == nil
}

func Load(name string) (*AgentConfig, error) {
	data, err := os.ReadFile(filepath.Join(Dir(name), "oc-agent.yaml"))
	if err != nil {
		return nil, fmt.Errorf("agent '%s' not found: %w", name, err)
	}
	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse agent config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *AgentConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal agent config: %w", err)
	}
	return os.WriteFile(filepath.Join(Dir(cfg.Name), "oc-agent.yaml"), data, 0644)
}

func Create(cfg AgentConfig) error {
	return CreateWithInstructions(cfg, "")
}

func CreateWithInstructions(cfg AgentConfig, instructions string) error {
	if err := ValidateName(cfg.Name); err != nil {
		return err
	}
	if Exists(cfg.Name) {
		return fmt.Errorf("agent '%s' already exists", cfg.Name)
	}

	dir := Dir(cfg.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create agent directory: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal agent config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "oc-agent.yaml"), data, 0644); err != nil {
		return err
	}

	agentMD := instructions
	if agentMD == "" {
		agentMD = fmt.Sprintf("# %s\n\nAdd instructions for your agent here.\n\nThis file is loaded as context when you spawn a session.\n", cfg.Name)
	}
	return os.WriteFile(filepath.Join(dir, "agent.md"), []byte(agentMD+"\n"), 0644)
}

func Remove(name string) error {
	dir := Dir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("agent '%s' not found", name)
	}
	return os.RemoveAll(dir)
}

func List() ([]AgentConfig, error) {
	entries, err := os.ReadDir(config.AgentsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var agents []AgentConfig
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cfg, err := Load(entry.Name())
		if err != nil {
			continue
		}
		agents = append(agents, *cfg)
	}
	return agents, nil
}
