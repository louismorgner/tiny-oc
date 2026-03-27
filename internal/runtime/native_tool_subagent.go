package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/session"
)

// SubAgentSpawnResult is the result of a sub-agent spawn operation.
type SubAgentSpawnResult struct {
	SessionID string
}

// SubAgentSpawnFunc is a callback that spawns a sub-agent session. It is injected
// by the caller to avoid an import cycle between runtime and spawn packages.
type SubAgentSpawnFunc func(agentName, prompt, workspace, parentSessionID string) (*SubAgentSpawnResult, error)

func nativeSubAgent(ctx nativeToolContext, call ToolCall) toolExecution {
	var args struct {
		Action    string `json:"action"`
		Agent     string `json:"agent"`
		Prompt    string `json:"prompt"`
		SessionID string `json:"session_id"`
		Partial   bool   `json:"partial"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("SubAgent", "", "", err)
	}

	switch args.Action {
	case "list":
		return subAgentList(ctx)
	case "spawn":
		return subAgentSpawn(ctx, args.Agent, args.Prompt)
	case "status":
		return subAgentStatus(ctx, args.SessionID)
	case "output":
		return subAgentOutput(ctx, args.SessionID, args.Partial)
	case "cancel":
		return subAgentCancel(ctx, args.SessionID)
	default:
		return toolFailure("SubAgent", "", "", fmt.Errorf("unknown action %q — must be one of: list, spawn, status, output, cancel", args.Action))
	}
}

// subAgentList returns the list of agents available to spawn as sub-agents.
func subAgentList(ctx nativeToolContext) toolExecution {
	rctx := runtimeContextFromNative(ctx)

	manifest := ctx.Manifest
	useManifest := manifest != nil

	if useManifest {
		if len(manifest.SubAgents) == 0 {
			return toolSuccess("SubAgent", "", "No sub-agent permissions configured for this session.", Step{
				Type:    "tool",
				Tool:    "SubAgent",
				Content: "list",
				Success: boolPtr(true),
			})
		}
	} else {
		parentCfg, err := rctx.LoadAgentConfig()
		if err != nil {
			return toolFailure("SubAgent", "", "", fmt.Errorf("failed to load agent config: %w", err))
		}
		if !parentCfg.CanSpawnAny() {
			return toolSuccess("SubAgent", "", "No sub-agent permissions configured.", Step{
				Type:    "tool",
				Tool:    "SubAgent",
				Content: "list",
				Success: boolPtr(true),
			})
		}
	}

	allAgents, err := rctx.ListAgents()
	if err != nil {
		return toolFailure("SubAgent", "", "", fmt.Errorf("failed to list agents: %w", err))
	}

	type agentInfo struct {
		Name        string `json:"name"`
		Model       string `json:"model,omitempty"`
		Description string `json:"description,omitempty"`
	}
	var result []agentInfo
	for _, a := range allAgents {
		allowed := false
		if useManifest {
			allowed = a.Name != ctx.Agent && CanSpawnFromManifest(manifest, a.Name)
		} else {
			// Re-load parent config for permission check (already validated above)
			parentCfg, _ := rctx.LoadAgentConfig()
			if parentCfg != nil {
				allowed = a.Name != ctx.Agent && parentCfg.CanSpawn(a.Name)
			}
		}
		if allowed {
			result = append(result, agentInfo{Name: a.Name, Model: a.Model, Description: a.Description})
		}
	}

	if len(result) == 0 {
		return toolSuccess("SubAgent", "", "[]", Step{
			Type:    "tool",
			Tool:    "SubAgent",
			Content: "list",
			Success: boolPtr(true),
		})
	}

	data, err := json.Marshal(result)
	if err != nil {
		return toolFailure("SubAgent", "", "", err)
	}

	return toolSuccess("SubAgent", "", string(data), Step{
		Type:    "tool",
		Tool:    "SubAgent",
		Content: "list",
		Success: boolPtr(true),
	})
}

// subAgentSpawn spawns a sub-agent in the background and returns its session ID.
func subAgentSpawn(ctx nativeToolContext, agentName, prompt string) toolExecution {
	if agentName == "" {
		return toolFailure("SubAgent", "", "", fmt.Errorf("agent name is required for spawn action"))
	}
	if prompt == "" {
		return toolFailure("SubAgent", "", "", fmt.Errorf("prompt is required for spawn action"))
	}
	if ctx.SpawnFunc == nil {
		return toolFailure("SubAgent", "", "", fmt.Errorf("sub-agent spawning is not available in this runtime context"))
	}

	// Check spawn permissions
	manifest := ctx.Manifest
	if manifest != nil {
		if !CanSpawnFromManifest(manifest, agentName) {
			return toolFailure("SubAgent", "", "", fmt.Errorf("agent '%s' is not allowed to spawn '%s' for this session", ctx.Agent, agentName))
		}
	} else {
		rctx := runtimeContextFromNative(ctx)
		parentCfg, err := rctx.LoadAgentConfig()
		if err != nil {
			return toolFailure("SubAgent", "", "", fmt.Errorf("failed to load parent agent config: %w", err))
		}
		if !parentCfg.CanSpawn(agentName) {
			return toolFailure("SubAgent", "", "", fmt.Errorf("agent '%s' is not allowed to spawn '%s' — check sub-agents config in oc-agent.yaml", ctx.Agent, agentName))
		}
	}

	result, err := ctx.SpawnFunc(agentName, prompt, ctx.Workspace, ctx.SessionID)
	if err != nil {
		return toolFailure("SubAgent", "", "", fmt.Errorf("failed to spawn sub-agent: %w", err))
	}

	_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.spawn", map[string]interface{}{
		"parent_agent":   ctx.Agent,
		"parent_session": ctx.SessionID,
		"target_agent":   agentName,
		"session_id":     result.SessionID,
		"prompt":         prompt,
		"source":         "native_tool",
	})

	response := map[string]string{
		"session_id": result.SessionID,
		"agent":      agentName,
		"status":     "spawned",
	}
	data, _ := json.Marshal(response)

	return toolSuccess("SubAgent", "", string(data), Step{
		Type:    "tool",
		Tool:    "SubAgent",
		Content: fmt.Sprintf("spawn %s", agentName),
		Success: boolPtr(true),
	})
}

// subAgentStatus returns the status of sub-agent sessions.
func subAgentStatus(ctx nativeToolContext, sessionID string) toolExecution {
	if sessionID == "" {
		// List all sub-agent statuses
		children, err := session.ListByParentInWorkspace(ctx.Workspace, ctx.SessionID)
		if err != nil {
			return toolFailure("SubAgent", "", "", fmt.Errorf("failed to list sub-agents: %w", err))
		}
		if len(children) == 0 {
			return toolSuccess("SubAgent", "", "[]", Step{
				Type:    "tool",
				Tool:    "SubAgent",
				Content: "status",
				Success: boolPtr(true),
			})
		}

		var result []map[string]interface{}
		for _, s := range children {
			entry := map[string]interface{}{
				"session_id": s.ID,
				"agent":      s.Agent,
				"status":     s.ResolvedStatus(),
			}
			if pendingQuestion, err := LoadPendingQuestion(&s); err == nil && pendingQuestion != nil {
				entry["pending_question"] = pendingQuestion
			}
			if s.Name != "" {
				entry["name"] = s.Name
			}
			if s.Prompt != "" {
				p := s.Prompt
				if len(p) > 100 {
					p = p[:97] + "..."
				}
				entry["prompt"] = p
			}
			result = append(result, entry)
		}
		data, _ := json.Marshal(result)
		return toolSuccess("SubAgent", "", string(data), Step{
			Type:    "tool",
			Tool:    "SubAgent",
			Content: "status",
			Success: boolPtr(true),
		})
	}

	// Specific session status
	s, err := session.FindByIDPrefixInWorkspace(ctx.Workspace, sessionID)
	if err != nil {
		return toolFailure("SubAgent", "", "", err)
	}
	if s.ParentSessionID != ctx.SessionID {
		return toolFailure("SubAgent", "", "", fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID))
	}

	status := s.ResolvedStatus()
	result := map[string]interface{}{
		"session_id": s.ID,
		"agent":      s.Agent,
		"status":     status,
	}
	if s.Name != "" {
		result["name"] = s.Name
	}
	if s.Prompt != "" {
		p := s.Prompt
		if len(p) > 100 {
			p = p[:97] + "..."
		}
		result["prompt"] = p
	}
	if pendingQuestion, err := LoadPendingQuestion(s); err == nil && pendingQuestion != nil {
		result["pending_question"] = pendingQuestion
	}
	if exitCode, err := s.ReadExitCode(); err == nil {
		result["exit_code"] = exitCode
	}
	data, _ := json.Marshal(result)
	return toolSuccess("SubAgent", "", string(data), Step{
		Type:    "tool",
		Tool:    "SubAgent",
		Content: fmt.Sprintf("status %s", sessionID),
		Success: boolPtr(true),
	})
}

// subAgentOutput reads the output of a sub-agent session.
func subAgentOutput(ctx nativeToolContext, sessionID string, partial bool) toolExecution {
	if sessionID == "" {
		return toolFailure("SubAgent", "", "", fmt.Errorf("session_id is required for output action"))
	}

	s, err := session.FindByIDPrefixInWorkspace(ctx.Workspace, sessionID)
	if err != nil {
		return toolFailure("SubAgent", "", "", err)
	}
	if s.ParentSessionID != ctx.SessionID {
		return toolFailure("SubAgent", "", "", fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID))
	}

	status := s.ResolvedStatus()

	// Try reading the final output file first
	outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
	data, err := os.ReadFile(outputPath)
	if err == nil {
		result := map[string]string{
			"session_id": s.ID,
			"status":     status,
			"output":     truncateString(string(data), maxToolOutputBytes-256),
		}
		out, _ := json.Marshal(result)
		return toolSuccess("SubAgent", "", string(out), Step{
			Type:    "tool",
			Tool:    "SubAgent",
			Content: fmt.Sprintf("output %s", sessionID),
			Success: boolPtr(true),
		})
	}

	if !os.IsNotExist(err) {
		return toolFailure("SubAgent", "", "", err)
	}

	// Final output doesn't exist — try partial output if requested or still active
	if partial || status == "active" {
		tmpPath := filepath.Join(s.WorkspacePath, "toc-output.txt.tmp")
		partialData, tmpErr := os.ReadFile(tmpPath)
		if tmpErr == nil && len(partialData) > 0 {
			result := map[string]string{
				"session_id": s.ID,
				"status":     status,
				"output":     truncateString(string(partialData), maxToolOutputBytes-256),
			}
			out, _ := json.Marshal(result)
			return toolSuccess("SubAgent", "", string(out), Step{
				Type:    "tool",
				Tool:    "SubAgent",
				Content: fmt.Sprintf("output %s", sessionID),
				Success: boolPtr(true),
			})
		}

		result := map[string]string{
			"session_id": s.ID,
			"status":     "running",
			"output":     "",
		}
		out, _ := json.Marshal(result)
		return toolSuccess("SubAgent", "", string(out), Step{
			Type:    "tool",
			Tool:    "SubAgent",
			Content: fmt.Sprintf("output %s", sessionID),
			Success: boolPtr(true),
		})
	}

	return toolFailure("SubAgent", "", "", fmt.Errorf("no output found for session '%s' (status: %s)", sessionID, status))
}

// subAgentCancel cancels a running sub-agent session.
func subAgentCancel(ctx nativeToolContext, sessionID string) toolExecution {
	if sessionID == "" {
		return toolFailure("SubAgent", "", "", fmt.Errorf("session_id is required for cancel action"))
	}

	s, err := session.FindByIDPrefixInWorkspace(ctx.Workspace, sessionID)
	if err != nil {
		return toolFailure("SubAgent", "", "", err)
	}
	if s.ParentSessionID != ctx.SessionID {
		return toolFailure("SubAgent", "", "", fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID))
	}

	status := s.ResolvedStatus()
	if status != "active" {
		return toolFailure("SubAgent", "", "", fmt.Errorf("cannot cancel session in '%s' state (only active sessions can be cancelled)", status))
	}

	pid, err := s.ReadPID()
	if err != nil {
		return toolFailure("SubAgent", "", "", fmt.Errorf("cannot read PID for session '%s': %w (session may predate PID tracking)", sessionID, err))
	}

	// Send SIGTERM to the process group (negative PID kills the group).
	killed := false
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			if err != syscall.ESRCH {
				return toolFailure("SubAgent", "", "", fmt.Errorf("failed to kill process %d: %w", pid, err))
			}
		} else {
			killed = true
		}
	} else {
		killed = true
	}

	// Write cancellation markers
	var persistErrors []error
	outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
	if _, statErr := os.Stat(outputPath); os.IsNotExist(statErr) {
		markerPath := filepath.Join(s.WorkspacePath, "toc-cancelled.txt")
		if writeErr := os.WriteFile(markerPath, []byte(fmt.Sprintf("cancelled by parent session %s\n", ctx.SessionID)), 0644); writeErr != nil {
			persistErrors = append(persistErrors, fmt.Errorf("write cancellation marker: %w", writeErr))
		}
	}
	if updateErr := session.UpdateStatusInWorkspace(ctx.Workspace, s.ID, session.StatusCancelled); updateErr != nil {
		persistErrors = append(persistErrors, fmt.Errorf("update session status: %w", updateErr))
	}
	state, loadErr := LoadState(s)
	if loadErr != nil && os.IsNotExist(loadErr) {
		state = &State{
			Runtime:    s.RuntimeName(),
			SessionID:  s.ID,
			Agent:      s.Agent,
			Workspace:  ctx.Workspace,
			SessionDir: s.WorkspacePath,
			Status:     session.StatusCancelled,
		}
		loadErr = nil
	}
	if loadErr == nil && state != nil {
		state.Status = session.StatusCancelled
		state.LastError = fmt.Sprintf("session cancelled by parent session %s", ctx.SessionID)
		if saveErr := SaveState(s, state); saveErr != nil {
			persistErrors = append(persistErrors, fmt.Errorf("save state: %w", saveErr))
		}
	}
	if appendErr := AppendEvent(s, Event{
		Timestamp: time.Now().UTC(),
		Step: Step{
			Type:    "error",
			Content: fmt.Sprintf("session cancelled by parent session %s", ctx.SessionID),
		},
	}); appendErr != nil {
		persistErrors = append(persistErrors, fmt.Errorf("append event: %w", appendErr))
	}

	_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.cancel", map[string]interface{}{
		"parent_session": ctx.SessionID,
		"session_id":     s.ID,
		"agent":          s.Agent,
		"pid":            pid,
		"killed":         killed,
		"source":         "native_tool",
	})

	if len(persistErrors) > 0 {
		result := map[string]interface{}{
			"session_id": s.ID,
			"status":     "cancelled",
			"warning":    fmt.Sprintf("cancelled but state persistence had errors: %s", errors.Join(persistErrors...)),
		}
		data, _ := json.Marshal(result)
		return toolSuccess("SubAgent", "", string(data), Step{
			Type:    "tool",
			Tool:    "SubAgent",
			Content: fmt.Sprintf("cancel %s", sessionID),
			Success: boolPtr(true),
		})
	}

	result := map[string]string{
		"session_id": s.ID,
		"status":     "cancelled",
	}
	data, _ := json.Marshal(result)
	return toolSuccess("SubAgent", "", string(data), Step{
		Type:    "tool",
		Tool:    "SubAgent",
		Content: fmt.Sprintf("cancel %s", sessionID),
		Success: boolPtr(true),
	})
}

// runtimeContextFromNative builds a runtime.Context from a nativeToolContext.
func runtimeContextFromNative(ctx nativeToolContext) *Context {
	return &Context{
		Workspace: ctx.Workspace,
		Agent:     ctx.Agent,
		SessionID: ctx.SessionID,
	}
}
