package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tiny-oc/toc/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
)

type Session struct {
	ID              string    `yaml:"id"`
	Agent           string    `yaml:"agent"`
	CreatedAt       time.Time `yaml:"created_at"`
	WorkspacePath   string    `yaml:"workspace_path"`
	Status          string    `yaml:"status,omitempty"`
	ParentSessionID string    `yaml:"parent_session_id,omitempty"`
	Prompt          string    `yaml:"prompt,omitempty"`
}

// ResolvedStatus returns the display status, checking workspace existence.
// For sub-agent sessions (ParentSessionID set), it also checks for toc-output.txt
// as a completion signal since the background process can't update sessions.yaml.
func (s *Session) ResolvedStatus() string {
	if _, err := os.Stat(s.WorkspacePath); os.IsNotExist(err) {
		return "stale"
	}
	if s.Status == StatusActive && s.ParentSessionID != "" {
		// Sub-agent: check if output file exists (means claude --print finished)
		if _, err := os.Stat(filepath.Join(s.WorkspacePath, "toc-output.txt")); err == nil {
			return "completed"
		}
	}
	if s.Status == StatusActive {
		return "active"
	}
	if s.Status == StatusCompleted {
		return "completed"
	}
	// Legacy sessions without status field
	return "completed"
}

// UpdateStatus sets the status of a session by ID.
func UpdateStatus(id, status string) error {
	sf, err := Load()
	if err != nil {
		return err
	}
	for i := range sf.Sessions {
		if sf.Sessions[i].ID == id {
			sf.Sessions[i].Status = status
			return Save(sf)
		}
	}
	return fmt.Errorf("session '%s' not found", id)
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
	return os.WriteFile(config.SessionsPath(), data, 0600)
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

func ListByParent(parentID string) ([]Session, error) {
	sf, err := Load()
	if err != nil {
		return nil, err
	}
	var result []Session
	for _, s := range sf.Sessions {
		if s.ParentSessionID == parentID {
			result = append(result, s)
		}
	}
	return result, nil
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

// FindByIDInWorkspace looks up a session by ID using a specific workspace path.
// AddInWorkspace adds a session record using a specific workspace path.
func AddInWorkspace(workspace string, s Session) error {
	path := workspace + "/.toc/sessions.yaml"
	data, err := os.ReadFile(path)
	var sf SessionsFile
	if err == nil {
		_ = yaml.Unmarshal(data, &sf)
	}
	sf.Sessions = append(sf.Sessions, s)
	out, err := yaml.Marshal(sf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// UpdateStatusInWorkspace updates session status using a specific workspace path.
func UpdateStatusInWorkspace(workspace, id, status string) error {
	path := workspace + "/.toc/sessions.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var sf SessionsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return err
	}
	for i := range sf.Sessions {
		if sf.Sessions[i].ID == id {
			sf.Sessions[i].Status = status
			out, err := yaml.Marshal(sf)
			if err != nil {
				return err
			}
			return os.WriteFile(path, out, 0644)
		}
	}
	return fmt.Errorf("session '%s' not found", id)
}

func FindByIDInWorkspace(workspace, id string) (*Session, error) {
	path := workspace + "/.toc/sessions.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session '%s' not found", id)
		}
		return nil, err
	}
	var sf SessionsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	for _, s := range sf.Sessions {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("session '%s' not found", id)
}

// ListByParentInWorkspace lists child sessions using a specific workspace path.
func ListByParentInWorkspace(workspace, parentID string) ([]Session, error) {
	path := workspace + "/.toc/sessions.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sf SessionsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	var result []Session
	for _, s := range sf.Sessions {
		if s.ParentSessionID == parentID {
			result = append(result, s)
		}
	}
	return result, nil
}
