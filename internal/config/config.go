package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const tocDir = ".toc"
const configFile = "config.yaml"
const secretsFile = "secrets.yaml"

type WorkspaceConfig struct {
	Name string `yaml:"name"`
}

// Secrets holds sensitive values like API keys, stored separately from config.
type Secrets struct {
	OpenRouterKey string `yaml:"openrouter_key,omitempty"`
}

func TocDir() string {
	return tocDir
}

func ConfigPath() string {
	return filepath.Join(tocDir, configFile)
}

func SecretsPath() string {
	return filepath.Join(tocDir, secretsFile)
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

// LoadSecrets reads the secrets file. Returns an empty Secrets if the file does not exist.
func LoadSecrets() (*Secrets, error) {
	data, err := os.ReadFile(SecretsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Secrets{}, nil
		}
		return nil, fmt.Errorf("failed to read secrets: %w", err)
	}
	var s Secrets
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}
	return &s, nil
}

// SaveSecrets writes the secrets file with restricted permissions (0600).
func SaveSecrets(s *Secrets) error {
	if err := os.MkdirAll(tocDir, 0755); err != nil {
		return fmt.Errorf("failed to create toc directory: %w", err)
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal secrets: %w", err)
	}
	return os.WriteFile(SecretsPath(), data, 0600)
}

// OpenRouterKey returns the stored OpenRouter API key, or empty string if not set.
func OpenRouterKey() string {
	s, err := LoadSecrets()
	if err != nil {
		return ""
	}
	return s.OpenRouterKey
}
