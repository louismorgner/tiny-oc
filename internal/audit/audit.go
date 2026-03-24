package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/tiny-oc/toc/internal/config"
)

// Event represents a single audit log entry.
type Event struct {
	Timestamp string                 `json:"ts"`
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"`
	Hostname  string                 `json:"hostname"`
	Workspace string                 `json:"workspace,omitempty"`
	Cwd       string                 `json:"cwd"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Version   string                 `json:"version"`
}

var (
	actor    string
	hostname string
	initOnce sync.Once
)

func initIdentity() {
	initOnce.Do(func() {
		actor = os.Getenv("USER")
		if actor == "" {
			actor = "unknown"
		}
		hostname, _ = os.Hostname()
		if hostname == "" {
			hostname = "unknown"
		}
	})
}

// Log appends an audit event to .toc/audit.log.
// Returns nil silently if the workspace is not initialized.
func Log(action string, details map[string]interface{}) error {
	if !config.Exists() {
		// Special case: workspace.init logs before config exists,
		// but the .toc directory should exist by the time we log.
		if action != "workspace.init" {
			return nil
		}
	}

	initIdentity()

	cwd, _ := os.Getwd()

	workspace := ""
	if cfg, err := config.Load(); err == nil {
		workspace = cfg.Name
	}

	event := Event{
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Action:    action,
		Actor:     actor,
		Hostname:  hostname,
		Workspace: workspace,
		Cwd:       cwd,
		Details:   details,
		Version:   version,
	}

	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(config.AuditLogPath(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(line)
	return err
}

// Read returns events from the audit log. If n > 0, returns the last n events.
// If action is non-empty, filters by action prefix.
func Read(n int, action string) ([]Event, error) {
	f, err := os.Open(config.AuditLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		if action != "" && !matchActionPrefix(e.Action, action) {
			continue
		}
		events = append(events, e)
	}

	if n > 0 && len(events) > n {
		events = events[len(events)-n:]
	}

	return events, scanner.Err()
}

func matchActionPrefix(eventAction, prefix string) bool {
	if len(eventAction) < len(prefix) {
		return false
	}
	return eventAction[:len(prefix)] == prefix
}

// version is set by the cmd package at init time.
var version = "dev"

// SetVersion sets the version string included in audit events.
func SetVersion(v string) {
	version = v
}
