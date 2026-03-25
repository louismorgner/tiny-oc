package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tiny-oc/toc/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	StatusActive         = "active"
	StatusCompleted      = "completed"
	StatusCompletedOK    = "completed-success"
	StatusCompletedError = "completed-error"
	StatusZombie         = "zombie"
	StatusCancelled      = "cancelled"
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

// ResolvedStatus returns the display status, checking filesystem signals.
// For sub-agent sessions it checks PID liveness, exit code, and cancellation markers
// since the background process can't update sessions.yaml directly.
func (s *Session) ResolvedStatus() string {
	if _, err := os.Stat(s.WorkspacePath); os.IsNotExist(err) {
		return "stale"
	}

	if s.Status == StatusActive && s.ParentSessionID != "" {
		return s.resolveSubAgentStatus()
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

// resolveSubAgentStatus determines the status of a sub-agent session by
// inspecting filesystem markers written by the wrapper script.
func (s *Session) resolveSubAgentStatus() string {
	// Check cancellation marker first
	if _, err := os.Stat(filepath.Join(s.WorkspacePath, "toc-cancelled.txt")); err == nil {
		return StatusCancelled
	}

	// Check if output file exists (means the process completed and mv succeeded)
	outputExists := false
	if _, err := os.Stat(filepath.Join(s.WorkspacePath, "toc-output.txt")); err == nil {
		outputExists = true
	}

	if outputExists {
		// Process finished — check exit code for success vs error
		exitCode, err := s.ReadExitCode()
		if err == nil {
			if exitCode == 0 {
				return StatusCompletedOK
			}
			return StatusCompletedError
		}
		// Exit code file missing (legacy session) — fall back to "completed"
		return "completed"
	}

	// Output file doesn't exist — check if process is still alive
	pid, err := s.ReadPID()
	if err != nil {
		// No PID file — legacy session or hasn't started yet
		return "active"
	}

	if isProcessAlive(pid) {
		return "active"
	}

	// Process is dead but never wrote output — zombie
	return StatusZombie
}

// ReadPID reads the process ID from toc-pid.txt in the session workspace.
func (s *Session) ReadPID() (int, error) {
	data, err := os.ReadFile(filepath.Join(s.WorkspacePath, "toc-pid.txt"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// ReadExitCode reads the exit code from toc-exit-code.txt in the session workspace.
func (s *Session) ReadExitCode() (int, error) {
	data, err := os.ReadFile(filepath.Join(s.WorkspacePath, "toc-exit-code.txt"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// isProcessAlive checks if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// UpdateStatus sets the status of a session by ID.
func UpdateStatus(id, status string) error {
	return withFileLock(config.SessionsPath(), func() error {
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
	})
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
	return withFileLock(config.SessionsPath(), func() error {
		sf, err := Load()
		if err != nil {
			return err
		}
		sf.Sessions = append(sf.Sessions, s)
		return Save(sf)
	})
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
	return withFileLock(config.SessionsPath(), func() error {
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
	})
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
	return withFileLock(path, func() error {
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
		return os.WriteFile(path, out, 0600)
	})
}

// UpdateStatusInWorkspace updates session status using a specific workspace path.
func UpdateStatusInWorkspace(workspace, id, status string) error {
	path := workspace + "/.toc/sessions.yaml"
	return withFileLock(path, func() error {
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
				return os.WriteFile(path, out, 0600)
			}
		}
		return fmt.Errorf("session '%s' not found", id)
	})
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

// FindByIDPrefix finds a session whose ID starts with the given prefix.
// Returns an error if no match or multiple matches are found.
func FindByIDPrefix(prefix string) (*Session, error) {
	sf, err := Load()
	if err != nil {
		return nil, err
	}
	var match *Session
	for i, s := range sf.Sessions {
		if len(s.ID) >= len(prefix) && s.ID[:len(prefix)] == prefix {
			if match != nil {
				return nil, fmt.Errorf("ambiguous session prefix '%s': matches multiple sessions", prefix)
			}
			match = &sf.Sessions[i]
		}
	}
	if match == nil {
		return nil, fmt.Errorf("session '%s' not found", prefix)
	}
	return match, nil
}

// FindByIDPrefixInWorkspace finds a session by ID prefix in a specific workspace.
func FindByIDPrefixInWorkspace(workspace, prefix string) (*Session, error) {
	path := workspace + "/.toc/sessions.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session '%s' not found", prefix)
	}
	var sf SessionsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	var match *Session
	for i, s := range sf.Sessions {
		if len(s.ID) >= len(prefix) && s.ID[:len(prefix)] == prefix {
			if match != nil {
				return nil, fmt.Errorf("ambiguous session prefix '%s': matches multiple sessions", prefix)
			}
			match = &sf.Sessions[i]
		}
	}
	if match == nil {
		return nil, fmt.Errorf("session '%s' not found", prefix)
	}
	return match, nil
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
