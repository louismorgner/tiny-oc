package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ComposePrompt renders the runtime-neutral instruction payload from agent.md
// and any compose files in the session workspace.
func ComposePrompt(workDir string, cfg *SessionConfig, sessionID string) (string, error) {
	var parts []string

	agentMD := filepath.Join(workDir, "agent.md")
	if data, err := os.ReadFile(agentMD); err == nil {
		parts = append(parts, strings.TrimSpace(string(data)))
	} else if !os.IsNotExist(err) {
		return "", err
	}

	for _, file := range cfg.Compose {
		path := filepath.Join(workDir, file)
		data, err := os.ReadFile(path)
		if err == nil {
			parts = append(parts, strings.TrimSpace(string(data)))
			continue
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}

	if len(parts) == 0 {
		return "", nil
	}

	content := strings.Join(parts, "\n\n---\n\n")
	now := time.Now()
	replacer := strings.NewReplacer(
		"{{.AgentName}}", cfg.Agent,
		"{{.SessionID}}", sessionID,
		"{{.Date}}", now.Format("2006-01-02"),
		"{{.Model}}", cfg.Model,
	)
	return replacer.Replace(content), nil
}
