package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tiny-oc/toc/internal/session"
)

const (
	SessionNotificationVersion          = 1
	SessionNotificationTypeSubAgentDone = "subagent.completed"
	maxNotificationOutputBytes          = 16 * 1024
)

type SessionNotification struct {
	Version         int       `json:"version"`
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	ParentSessionID string    `json:"parent_session_id"`
	SessionID       string    `json:"session_id"`
	Agent           string    `json:"agent"`
	Status          string    `json:"status"`
	ExitCode        int       `json:"exit_code,omitempty"`
	Prompt          string    `json:"prompt,omitempty"`
	Output          string    `json:"output,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func sessionNotificationDir(workspace, sessionID string) string {
	return filepath.Join(MetadataDir(workspace, sessionID), "notifications")
}

func WriteSessionNotification(workspace, sessionID string, notification SessionNotification) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("session notification requires a parent session ID")
	}
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}
	if notification.Version == 0 {
		notification.Version = SessionNotificationVersion
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now().UTC()
	}

	dir := sessionNotificationDir(workspace, sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%020d-%s.json", notification.CreatedAt.UnixNano(), notification.ID)
	finalPath := filepath.Join(dir, filename)
	tmpPath := finalPath + ".tmp"

	data, err := json.MarshalIndent(notification, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0600); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return notification.ID, nil
}

func PopSessionNotification(workspace, sessionID string) (*SessionNotification, error) {
	dir := sessionNotificationDir(workspace, sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		var notification SessionNotification
		if err := json.Unmarshal(data, &notification); err != nil {
			_ = os.Remove(path)
			return nil, err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return &notification, nil
	}

	return nil, nil
}

func WaitForSessionNotification(workspace, sessionID string, timeout time.Duration) (*SessionNotification, error) {
	deadline := time.Now().Add(timeout)
	for {
		notification, err := PopSessionNotification(workspace, sessionID)
		if err != nil {
			return nil, err
		}
		if notification != nil {
			return notification, nil
		}
		if timeout > 0 && time.Now().After(deadline) {
			return nil, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func WriteSubAgentCompletionNotification(workspace, parentSessionID string, notification SessionNotification) (string, error) {
	notification.Type = SessionNotificationTypeSubAgentDone
	notification.ParentSessionID = parentSessionID
	notification.Output = truncateNotificationOutput(notification.Output)
	return WriteSessionNotification(workspace, parentSessionID, notification)
}

func HasActiveSubAgents(workspace, parentSessionID string) (bool, error) {
	children, err := session.ListByParentInWorkspace(workspace, parentSessionID)
	if err != nil {
		return false, err
	}
	for _, child := range children {
		if child.ResolvedStatus() == session.StatusActive {
			return true, nil
		}
	}
	return false, nil
}

func SessionNotificationPrompt(notification SessionNotification) string {
	status := strings.TrimSpace(notification.Status)
	if status == "" {
		status = "completed"
	}

	var b strings.Builder
	b.WriteString("Sub-agent completion update.\n\n")
	b.WriteString(fmt.Sprintf("Agent: %s\n", notification.Agent))
	b.WriteString(fmt.Sprintf("Session: %s\n", notification.SessionID))
	b.WriteString(fmt.Sprintf("Status: %s\n", status))
	if notification.ExitCode != 0 || status == session.StatusCompletedError {
		b.WriteString(fmt.Sprintf("Exit code: %d\n", notification.ExitCode))
	}
	if strings.TrimSpace(notification.Prompt) != "" {
		b.WriteString("\nOriginal task:\n")
		b.WriteString(notification.Prompt)
		b.WriteString("\n")
	}
	if strings.TrimSpace(notification.Output) != "" {
		b.WriteString("\nOutput:\n")
		b.WriteString(notification.Output)
		b.WriteString("\n")
	}
	b.WriteString("\nContinue the main task using this result.")
	return b.String()
}

func truncateNotificationOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= maxNotificationOutputBytes {
		return output
	}
	return strings.TrimSpace(output[:maxNotificationOutputBytes]) + "\n...[truncated]"
}
