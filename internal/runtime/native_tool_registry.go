package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
)

type nativeToolHandler func(nativeToolContext, ToolCall) toolExecution

type NativeToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Handler     nativeToolHandler
	// Deferred indicates this tool's full schema is not sent with every
	// request. Instead, only its name and a one-line summary are listed
	// in the system prompt. The model fetches the full schema on demand
	// via the ToolSearch meta-tool. This saves significant tokens per
	// request for tools that are used infrequently.
	Deferred bool
	// Summary is a one-line description used in the deferred tool list.
	// Only needed when Deferred is true.
	Summary string
}

func nativeToolRegistry() []NativeToolSpec {
	return []NativeToolSpec{
		{
			Name: "Read",
			Description: `Read a file from the session workspace. Use instead of cat/head/tail via Bash. MUST Read before Edit.

- file_path (required): Relative or absolute path within the workspace.
- start_line / end_line (optional): 1-based line range for partial reads.

Output truncated at 64KB. Use Glob for directories, start_line/end_line for large files.`,
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
			Name: "Write",
			Description: `Create or overwrite a file in the session workspace. For targeted edits, prefer Edit instead.

- file_path (required): Path within the workspace. Parent dirs created automatically.
- content (required): Full file content (replaces entire file).

Preserves existing file permissions. Do not write secrets without user confirmation.`,
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
			Name: "Edit",
			Description: `Exact string replacement in a file. You MUST Read the file first.

- file_path (required): Path within the workspace.
- old_string (required): Exact text to find (including whitespace/indentation).
- new_string (required): Replacement text. Empty to delete.
- replace_all (optional, default false): If false and multiple matches exist, the edit fails. Set true to replace all.

Fails if old_string not found or matches multiple times without replace_all.`,
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
			Name:     "Glob",
			Deferred: true,
			Summary:  "Find files by name pattern using glob matching",
			Description: `Find files by name pattern within the session workspace using glob matching.

Use Glob to discover files when you know the naming pattern but not the exact location. It walks the directory tree and returns paths matching the pattern. Use Glob instead of running find or ls via Bash.

The search automatically skips .git and .toc-native directories. Results are sorted alphabetically and deduplicated.

Parameters:
- pattern (required): Glob pattern to match against relative file paths (e.g., "**/*.go", "src/**/*.ts", "*.yaml"). Uses standard glob syntax with ** for recursive matching.
- path (optional): Subdirectory to search within. Defaults to the workspace root if omitted. Must be within the workspace.

Output: A newline-separated list of matching file paths (relative to the search root). Empty output means no files matched.

Anti-patterns:
- Do NOT use Bash with find or ls to search for files — use Glob instead.
- Do NOT use Glob to search file contents — use Grep for that.
- For very broad patterns like "**/*", consider narrowing with a path to avoid large result sets.`,
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
			Name:     "Grep",
			Deferred: true,
			Summary:  "Search file contents using ripgrep regex patterns",
			Description: `Search file contents within the session workspace using ripgrep (rg).

Use Grep to find patterns in file contents. It supports full regular expression syntax. Use Grep instead of running grep or rg via Bash — it is pre-configured to search the workspace correctly, skipping .git and .toc-native directories, and including hidden files.

Parameters:
- pattern (required): Regular expression pattern to search for (ripgrep syntax). Examples: "func main", "TODO|FIXME", "import\s+\(".  Literal special characters like braces need escaping: use "interface\{\}" to find "interface{}".
- path (optional): File or subdirectory to search within. Defaults to the entire workspace if omitted. Must be within the workspace.

Output: Matching lines with file paths and line numbers in the format "path:line:content", one match per line. Output is truncated at 64KB. If no matches are found, output is empty (this is not an error).

Anti-patterns:
- Do NOT use Bash to run grep or rg — use this tool instead.
- Do NOT use Grep to find files by name — use Glob for that.
- For searching a single known file, consider using Read with start_line/end_line if you know the approximate location.`,
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
			Name: "Bash",
			Description: `Execute a shell command via "sh -lc" in the session workspace. Use for builds, tests, git, installs.

- command (required): Shell command to execute.
- timeout_ms (optional): Timeout in ms (default 30000). Increase for long builds/tests.

Output: Combined stdout+stderr, truncated at 64KB. Includes exit code on failure.
Prefer Read/Write/Edit/Glob/Grep over Bash for file operations. Do not run interactive commands.`,
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
			Name:     "Skill",
			Deferred: true,
			Summary:  "Load full instructions for a named session skill",
			Description: `Load the full instructions for a named skill provisioned in this session.

Available skills are listed in the system prompt under <available_skills>. When a task matches a skill's description, call this tool with the skill's name to load its complete instructions, then follow them.

Parameters:
- skill (required): The name of the skill to load, as listed in the system prompt.

Output: The full content of the skill's SKILL.md file (truncated at 64KB). Returns an error if the skill does not exist.

Anti-patterns:
- Do NOT guess skill names — only use names listed in <available_skills>.
- Do NOT invoke a skill that is already loaded in the current conversation — follow its instructions directly instead of loading it again.`,
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
			Name:     "SubAgent",
			Deferred: true,
			Summary:  "Manage sub-agent sessions for parallel task execution",
			Description: `Manage sub-agent sessions for multi-agent orchestration. Sub-agents run in the background as separate sessions with their own workspace, allowing parallel task execution.

Use SubAgent when a task is self-contained, has a clear deliverable, and can run independently. Do NOT use SubAgent when you need tight back-and-forth iteration — do the work yourself instead.

Actions:
- "list": List agents available to spawn. Returns a JSON array with each agent's name, model, and description. Use this first to discover what agents you can delegate to. No additional parameters needed.
- "spawn": Start a sub-agent in the background. Requires "agent" (must be an agent name from list) and "prompt" (a clear, complete task description). Returns a JSON object with session_id, agent name, and status. The sub-agent runs asynchronously — use status/output to track it.
- "status": Check progress. Omit "session_id" to get status of ALL sub-agents for this session. Provide "session_id" to check a specific one. Returns JSON with session_id, agent, status (active/completed/failed/cancelled), and a truncated prompt.
- "output": Read the result of a sub-agent. Requires "session_id". Returns JSON with session_id, status, and output text. Set "partial" to true to read intermediate output from a still-running session. If the session is complete but has no output file, returns an error.
- "cancel": Terminate a running sub-agent. Requires "session_id". Only works on sessions with "active" status. Sends SIGTERM to the process and marks the session as cancelled.

Output: All actions return JSON. Errors include descriptive messages about what went wrong (permission denied, session not found, invalid state, etc.).

Anti-patterns:
- Do NOT spawn sub-agents for trivial tasks you can do faster yourself.
- Do NOT poll status in a tight loop — give sub-agents time to work before checking.
- Do NOT forget to read output after a sub-agent completes — the result is your deliverable.
- Do NOT spawn agents you are not permitted to use — call "list" first to see what is available.`,
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

// toolSearchSpec returns the ToolSearch meta-tool that lets the model
// fetch full schemas for deferred tools on demand. This is always
// included in the active tool set (never deferred itself).
func toolSearchSpec(allSpecs []NativeToolSpec) NativeToolSpec {
	return NativeToolSpec{
		Name: "ToolSearch",
		Description: `Fetch full schema definitions for deferred tools so they can be called.

Deferred tools appear by name in the system prompt's deferred tool list. Until fetched, only the name and one-line summary are known — the tool cannot be invoked. Call ToolSearch with a comma-separated list of tool names to retrieve their full definitions.

Parameters:
- tools (required): Comma-separated tool names to fetch (e.g., "Glob,Grep").

Output: Full tool definitions (name, description, parameters) as JSON for each matched tool. Once fetched, the tool can be called normally.`,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tools": map[string]interface{}{"type": "string", "description": "Comma-separated tool names to fetch schemas for"},
			},
			"required": []string{"tools"},
		},
		Handler: makeToolSearchHandler(allSpecs),
	}
}

// makeToolSearchHandler creates a ToolSearch handler that has access to
// the full tool registry for schema lookups.
func makeToolSearchHandler(allSpecs []NativeToolSpec) nativeToolHandler {
	return func(ctx nativeToolContext, call ToolCall) toolExecution {
		var args struct {
			Tools string `json:"tools"`
		}
		if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
			return toolFailure("ToolSearch", "", "", err)
		}
		if strings.TrimSpace(args.Tools) == "" {
			return toolFailure("ToolSearch", "", "", fmt.Errorf("tools parameter is required"))
		}

		requested := strings.Split(args.Tools, ",")
		var results []map[string]interface{}
		for _, name := range requested {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			for _, spec := range allSpecs {
				if spec.Name == name {
					results = append(results, map[string]interface{}{
						"name":        spec.Name,
						"description": spec.Description,
						"parameters":  spec.Parameters,
					})
					break
				}
			}
		}

		if len(results) == 0 {
			return toolFailure("ToolSearch", "", args.Tools, fmt.Errorf("no matching tools found for: %s", args.Tools))
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return toolFailure("ToolSearch", "", "", err)
		}
		return toolSuccess("ToolSearch", "", string(data), Step{
			Type:    "tool",
			Tool:    "ToolSearch",
			Content: args.Tools,
			Success: boolPtr(true),
		})
	}
}

// DeferredToolSummary returns a formatted list of deferred tools for
// injection into the system prompt, so the model knows what's available.
func DeferredToolSummary(specs []NativeToolSpec) string {
	var deferred []NativeToolSpec
	for _, spec := range specs {
		if spec.Deferred {
			deferred = append(deferred, spec)
		}
	}
	if len(deferred) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The following tools are available but their full schemas are deferred. Use the ToolSearch tool to fetch their full definitions before calling them.\n\n")
	b.WriteString("<deferred_tools>\n")
	for _, spec := range deferred {
		summary := spec.Summary
		if summary == "" {
			summary = spec.Name
		}
		fmt.Fprintf(&b, "  <tool name=%q>%s</tool>\n", spec.Name, summary)
	}
	b.WriteString("</deferred_tools>")
	return b.String()
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
	// Append the ToolSearch meta-tool so the model can fetch deferred schemas.
	registry = append(registry, toolSearchSpec(registry))

	if len(enabled) == 0 {
		return registry
	}

	allowed := make(map[string]bool, len(enabled))
	for _, name := range enabled {
		allowed[name] = true
	}
	// ToolSearch is always allowed when deferred tools are in play.
	allowed["ToolSearch"] = true

	result := make([]NativeToolSpec, 0, len(enabled)+1)
	for _, spec := range registry {
		if allowed[spec.Name] {
			result = append(result, spec)
		}
	}
	return result
}

// nativeToolDefinitions returns API tool definitions, omitting full schemas
// for deferred tools. Deferred tools are listed in the system prompt instead
// and their schemas are fetched on demand via ToolSearch.
func nativeToolDefinitions(specs []NativeToolSpec) []toolDefinition {
	defs := make([]toolDefinition, 0, len(specs))
	for _, spec := range specs {
		if spec.Deferred {
			// Don't send full schema for deferred tools — the model
			// uses ToolSearch to fetch them when needed.
			continue
		}
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
