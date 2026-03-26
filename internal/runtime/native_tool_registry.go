package runtime

import "fmt"

type nativeToolHandler func(nativeToolContext, ToolCall) toolExecution

type NativeToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Handler     nativeToolHandler
}

func nativeToolRegistry() []NativeToolSpec {
	return []NativeToolSpec{
		{
			Name:        "Read",
			Description: "Read a file from the current session workspace.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path":  map[string]interface{}{"type": "string"},
					"start_line": map[string]interface{}{"type": "integer"},
					"end_line":   map[string]interface{}{"type": "integer"},
				},
				"required": []string{"file_path"},
			},
			Handler: nativeRead,
		},
		{
			Name:        "Write",
			Description: "Write a full file in the current session workspace.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{"type": "string"},
					"content":   map[string]interface{}{"type": "string"},
				},
				"required": []string{"file_path", "content"},
			},
			Handler: nativeWrite,
		},
		{
			Name:        "Edit",
			Description: "Replace text in a file in the current session workspace.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path":   map[string]interface{}{"type": "string"},
					"old_string":  map[string]interface{}{"type": "string"},
					"new_string":  map[string]interface{}{"type": "string"},
					"replace_all": map[string]interface{}{"type": "boolean"},
				},
				"required": []string{"file_path", "old_string", "new_string"},
			},
			Handler: nativeEdit,
		},
		{
			Name:        "Glob",
			Description: "Find files in the current session workspace using a glob-like pattern.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pattern": map[string]interface{}{"type": "string"},
					"path":    map[string]interface{}{"type": "string"},
				},
				"required": []string{"pattern"},
			},
			Handler: nativeGlob,
		},
		{
			Name:        "Grep",
			Description: "Search file contents in the current session workspace using ripgrep syntax.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pattern": map[string]interface{}{"type": "string"},
					"path":    map[string]interface{}{"type": "string"},
				},
				"required": []string{"pattern"},
			},
			Handler: nativeGrep,
		},
		{
			Name:        "Bash",
			Description: "Run a shell command in the current session workspace.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command":    map[string]interface{}{"type": "string"},
					"timeout_ms": map[string]interface{}{"type": "integer"},
				},
				"required": []string{"command"},
			},
			Handler: nativeBash,
		},
		{
			Name:        "Skill",
			Description: "Read the instructions for a provisioned skill in this session.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill": map[string]interface{}{"type": "string"},
				},
				"required": []string{"skill"},
			},
			Handler: nativeSkill,
		},
		{
			Name: "SubAgent",
			Description: `Manage sub-agent sessions for multi-agent orchestration.

Actions:
- "list": List agents available to spawn as sub-agents. No additional parameters needed.
- "spawn": Spawn a sub-agent in the background. Requires "agent" (agent name) and "prompt" (task description). Returns a session_id to track progress.
- "status": Check sub-agent status. Optional "session_id" for a specific session; omit to list all sub-agents.
- "output": Read the output of a completed or running sub-agent. Requires "session_id". Set "partial" to true to read partial output from a still-running session.
- "cancel": Cancel a running sub-agent. Requires "session_id".`,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action":     map[string]interface{}{"type": "string", "description": "The action to perform: list, spawn, status, output, or cancel"},
					"agent":      map[string]interface{}{"type": "string", "description": "Agent name (required for spawn)"},
					"prompt":     map[string]interface{}{"type": "string", "description": "Task prompt for the sub-agent (required for spawn)"},
					"session_id": map[string]interface{}{"type": "string", "description": "Session ID (required for output and cancel, optional for status)"},
					"partial":    map[string]interface{}{"type": "boolean", "description": "Read partial output from a running session (for output action)"},
				},
				"required": []string{"action"},
			},
			Handler: nativeSubAgent,
		},
	}
}

func NativeToolNames() []string {
	registry := nativeToolRegistry()
	names := make([]string, 0, len(registry))
	for _, spec := range registry {
		names = append(names, spec.Name)
	}
	return names
}

func nativeToolSet(enabled []string) []NativeToolSpec {
	registry := nativeToolRegistry()
	if len(enabled) == 0 {
		return registry
	}

	allowed := make(map[string]bool, len(enabled))
	for _, name := range enabled {
		allowed[name] = true
	}

	result := make([]NativeToolSpec, 0, len(enabled))
	for _, spec := range registry {
		if allowed[spec.Name] {
			result = append(result, spec)
		}
	}
	return result
}

func nativeToolDefinitions(specs []NativeToolSpec) []toolDefinition {
	defs := make([]toolDefinition, 0, len(specs))
	for _, spec := range specs {
		defs = append(defs, toolDefinition{
			Type: "function",
			Function: toolDescriptor{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  spec.Parameters,
			},
		})
	}
	return defs
}

func executeNativeTool(specs []NativeToolSpec, ctx nativeToolContext, call ToolCall) toolExecution {
	for _, spec := range specs {
		if spec.Name == call.Function.Name {
			return spec.Handler(ctx, call)
		}
	}
	return toolFailure(call.Function.Name, "", "", fmt.Errorf("unknown or disabled tool: %s", call.Function.Name))
}
