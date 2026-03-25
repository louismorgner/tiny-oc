package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const tocDir = ".toc"
const configFile = "config.yaml"

type WorkspaceConfig struct {
	Name string `yaml:"name"`
}

func TocDir() string {
	return tocDir
}

func ConfigPath() string {
	return filepath.Join(tocDir, configFile)
}

func AgentsDir() string {
	return filepath.Join(tocDir, "agents")
}

func SkillsDir() string {
	return filepath.Join(tocDir, "skills")
}

func SkillsRegistryPath() string {
	return filepath.Join(tocDir, "skills.yaml")
}

func SessionsPath() string {
	return filepath.Join(tocDir, "sessions.yaml")
}

func IntegrationsDir() string {
	return filepath.Join(tocDir, "integrations")
}

func SessionsDir() string {
	return filepath.Join(tocDir, "sessions")
}

func AuditLogPath() string {
	return filepath.Join(tocDir, "audit.log")
}

func Exists() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

func Load() (*WorkspaceConfig, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	var cfg WorkspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *WorkspaceConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(ConfigPath(), data, 0644)
}

func EnsureInitialized() (*WorkspaceConfig, error) {
	if !Exists() {
		return nil, fmt.Errorf("this directory is not a toc workspace — run 'toc init' first")
	}
	return Load()
}

func Init(name string) error {
	if Exists() {
		return fmt.Errorf("workspace already initialized")
	}
	if err := os.MkdirAll(AgentsDir(), 0755); err != nil {
		return fmt.Errorf("failed to create agents directory: %w", err)
	}
	if err := os.MkdirAll(SkillsDir(), 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}
	return Save(&WorkspaceConfig{Name: name})
}
