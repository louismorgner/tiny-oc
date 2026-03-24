package session

import (
	"fmt"
	"os"
	"time"

	"github.com/tiny-oc/toc/internal/config"
	"gopkg.in/yaml.v3"
)

type Session struct {
	ID            string    `yaml:"id"`
	Agent         string    `yaml:"agent"`
	CreatedAt     time.Time `yaml:"created_at"`
	WorkspacePath string    `yaml:"workspace_path"`
}

type SessionsFile struct {
	Sessions []Session `yaml:"sessions"`
}

func Load() (*SessionsFile, error) {
	data, err := os.ReadFile(config.SessionsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionsFile{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions: %w", err)
	}
	var sf SessionsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse sessions: %w", err)
	}
	return &sf, nil
}

func Save(sf *SessionsFile) error {
	data, err := yaml.Marshal(sf)
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}
	return os.WriteFile(config.SessionsPath(), data, 0644)
}

func Add(s Session) error {
	sf, err := Load()
	if err != nil {
		return err
	}
	sf.Sessions = append(sf.Sessions, s)
	return Save(sf)
}

func ListByAgent(agentName string) ([]Session, error) {
	sf, err := Load()
	if err != nil {
		return nil, err
	}
	var result []Session
	for _, s := range sf.Sessions {
		if s.Agent == agentName {
			result = append(result, s)
		}
	}
	return result, nil
}

func RemoveByAgent(agentName string) error {
	sf, err := Load()
	if err != nil {
		return err
	}
	var kept []Session
	for _, s := range sf.Sessions {
		if s.Agent != agentName {
			kept = append(kept, s)
		}
	}
	sf.Sessions = kept
	return Save(sf)
}

func FindByID(id string) (*Session, error) {
	sf, err := Load()
	if err != nil {
		return nil, err
	}
	for _, s := range sf.Sessions {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("session '%s' not found", id)
}
