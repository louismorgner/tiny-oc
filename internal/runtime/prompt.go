package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
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
		"{{.SubAgentInstructions}}", subAgentInstructions(cfg.Runtime, cfg.Permissions),
	)
	return replacer.Replace(content), nil
}

// subAgentInstructions returns runtime-specific sub-agent guidance. If the
// agent has no sub-agent permissions the placeholder resolves to empty string
// so agent.md authors can include {{.SubAgentInstructions}} unconditionally.
func subAgentInstructions(runtimeName string, perms agent.Permissions) string {
	if !hasSubAgentPermissions(perms) {
		return ""
	}

	switch runtimeName {
	case runtimeinfo.NativeRuntime:
		return nativeSubAgentInstructions
	default:
		return claudeCodeSubAgentInstructions
	}
}

func hasSubAgentPermissions(perms agent.Permissions) bool {
	for _, level := range perms.SubAgents {
		if level != agent.PermOff {
			return true
		}
	}
	return false
}

const claudeCodeSubAgentInstructions = `## Sub-agents

You can delegate work to other agents in the workspace.

` + "```bash" + `
toc runtime list                                    # see what agents you can spawn
toc runtime spawn <agent> --prompt "task description"  # spawn in background
toc runtime status                                  # check all sub-agent progress
toc runtime output <session-id>                     # read completed output
` + "```" + `

Delegate when the task is self-contained and has a clear deliverable. Don't delegate when you need tight back-and-forth iteration — do it yourself.

Write detailed prompts. Include full context: URLs, specific feedback items, line references. The sub-agent has no context beyond what you give it.

Spawn multiple sub-agents in parallel when tasks are independent. Check status periodically, then read the output when complete.`

const nativeSubAgentInstructions = `## Sub-agents

You can delegate work to other agents in the workspace using the SubAgent tool.

**Workflow:**
1. Call SubAgent with action "list" to see available agents
2. Call SubAgent with action "spawn", providing the agent name and a detailed prompt
3. Call SubAgent with action "status" to check progress (omit session_id to check all)
4. Call SubAgent with action "output" with the session_id to read the result

Delegate when the task is self-contained and has a clear deliverable. Don't delegate when you need tight back-and-forth iteration — do it yourself.

Write detailed prompts. Include full context: URLs, specific feedback items, line references. The sub-agent has no context beyond what you give it.

Spawn multiple sub-agents in parallel when tasks are independent. Check status periodically, then read the output when complete.`
