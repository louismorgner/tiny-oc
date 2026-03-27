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
			Name: "Read",
			Description: `Read a file from the session workspace and return its contents.

Use this tool instead of running cat, head, or tail via Bash. Use Read whenever you need to inspect file contents before making changes. You MUST Read a file before using Edit on it.

Parameters:
- file_path (required): Path to the file. Can be relative (resolved against the session workspace root) or absolute (must be within the workspace). Paths that escape the workspace are rejected.
- start_line / end_line (optional): 1-based line range to read a subset of the file. Omit both to read the entire file. Useful for large files when you know which section you need.

Output: The raw file contents (or the requested line range). Output is truncated at 64KB if the file is larger. The tool returns an error if the file does not exist or the path escapes the workspace.

Anti-patterns:
- Do NOT use Bash with cat/head/tail/sed to read files — use this tool instead.
- Do NOT read entire large files when you only need a specific section — use start_line/end_line.
- Do NOT attempt to read directories — use Glob to list files or Bash with ls.`,
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
			Description: `Write a complete file to the session workspace, creating it if it does not exist or overwriting it entirely if it does.

Use Write to create new files or to completely replace file contents. For small, targeted changes to existing files, prefer Edit instead — it only sends the diff and avoids accidentally dropping content. Parent directories are created automatically if they do not exist. If the file already exists, its permission mode is preserved; new files are created with mode 0644.

Parameters:
- file_path (required): Path to the file. Can be relative or absolute, but must resolve within the session workspace.
- content (required): The full file content to write. This replaces the entire file.

Output: A confirmation message with the byte count written and the file path.

Anti-patterns:
- Do NOT use Write to make small edits to existing files — use Edit instead, which is safer and more efficient.
- Do NOT use Bash with echo/cat heredocs to write files — use this tool instead.
- Do NOT write files containing secrets (.env, credentials) without explicit user confirmation.`,
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
			Description: `Perform exact string replacement in a file within the session workspace.

Use Edit for targeted modifications to existing files. It finds old_string in the file and replaces it with new_string. This is safer than Write for modifications because it only changes what you specify, leaving the rest of the file untouched.

Precondition: You MUST Read the file before editing it, so you know the exact text to match. Guessing at old_string without reading the file first is the most common cause of edit failures.

Parameters:
- file_path (required): Path to the file to edit, relative or absolute within the workspace.
- old_string (required): The exact text to find. Must not be empty. Must match the file contents exactly, including whitespace and indentation.
- new_string (required): The replacement text. Can be empty to delete old_string.
- replace_all (optional, default false): If false and old_string matches more than once, the edit FAILS with an error showing the match count. Set to true to replace all occurrences — useful for renaming variables or updating repeated patterns.

Output: A confirmation message on success. On failure, an error explaining why (not found, multiple matches without replace_all, etc.).

Anti-patterns:
- Do NOT edit a file you have not Read first — you will guess wrong about the content.
- Do NOT use Bash with sed or awk for file edits — use this tool instead.
- If old_string matches multiple times and you only want to change one, include more surrounding context to make the match unique rather than using replace_all.`,
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
			Name: "Glob",
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
			Name: "Grep",
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
			Description: `Execute a shell command in the session workspace.

Use Bash for tasks that the other tools cannot handle: running builds, tests, git operations, installing dependencies, or any general-purpose command. The command runs via "sh -lc" (login shell), so user profile tools like nvm, rbenv, and custom PATH entries are available.

The working directory is set to the session workspace root. Commands cannot escape the workspace sandbox.

Parameters:
- command (required): The shell command to execute. Use absolute paths or paths relative to the workspace root. Always quote file paths containing spaces.
- timeout_ms (optional): Timeout in milliseconds. Defaults to 30 seconds (30000ms). Set higher for long-running builds or tests.

Output: Combined stdout and stderr from the command. If the command fails, the output includes the exit code. If it times out, the output says "command timed out" along with any partial output. Output is truncated at 64KB.

When to use other tools instead:
- To read files: use Read (not cat/head/tail)
- To write files: use Write (not echo/cat heredoc)
- To edit files: use Edit (not sed/awk)
- To find files by name: use Glob (not find/ls)
- To search file contents: use Grep (not grep/rg)

Anti-patterns:
- Do NOT run interactive commands (vim, less, git rebase -i) — they will hang and timeout.
- Do NOT run long-running servers without increasing timeout_ms — the default 30s will kill them.
- Do NOT use Bash for file operations when a dedicated tool exists — the dedicated tools are safer and produce structured output.`,
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
			Name: "Skill",
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
			Name: "TodoWrite",
			Description: `Replace the current session todo list with a new ordered list.

Use TodoWrite for multi-step work, especially when you need to track progress across file edits, tests, and sub-agent coordination. Each call replaces the entire list, so always send the full current todo list.

Parameters:
- todos (required): Complete ordered todo list. Each item must include:
  - content: brief task description
  - status: pending, in_progress, completed, or cancelled
  - priority: high, medium, or low

Guidelines:
- Prefer using TodoWrite when the task has multiple meaningful steps.
- Keep only one item in_progress at a time when possible.
- Mark items completed immediately after finishing them.
- If the plan changes, rewrite the full list to match the new reality.

Output: A short summary confirming the updated todo list.`,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"todos": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"content":  map[string]interface{}{"type": "string", "description": "Brief description of the task"},
								"status":   map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "completed", "cancelled"}},
								"priority": map[string]interface{}{"type": "string", "enum": []string{"high", "medium", "low"}},
							},
							"required": []string{"content", "status", "priority"},
						},
					},
				},
				"required": []string{"todos"},
			},
			Handler: nativeTodoWrite,
		},
		{
			Name: "SubAgent",
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
