package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/fileutil"
)

// MatchesAny checks if a relative file path matches any of the context patterns.
// Patterns follow filepath.Match syntax with added support for directory prefixes
// (patterns ending in /) and ** for recursive matching.
func MatchesAny(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(relPath, pattern) {
			return true
		}
	}
	return false
}

func matchPattern(relPath, pattern string) bool {
	pattern = filepath.Clean(pattern)

	// Directory pattern (e.g. "docs/") — match anything under it
	if strings.HasSuffix(pattern, "/") || isDir(pattern) {
		dir := strings.TrimSuffix(pattern, "/")
		if relPath == dir || strings.HasPrefix(relPath, dir+"/") {
			return true
		}
	}

	// Glob with ** support — expand to recursive match
	if strings.Contains(pattern, "**") {
		return matchDoublestar(relPath, pattern)
	}

	// Standard glob
	matched, _ := filepath.Match(pattern, relPath)
	if matched {
		return true
	}

	// Also try matching just the filename (e.g. "*.md" matches "foo/bar.md")
	if !strings.Contains(pattern, "/") {
		matched, _ = filepath.Match(pattern, filepath.Base(relPath))
		return matched
	}

	return false
}

func matchDoublestar(relPath, pattern string) bool {
	// Split pattern on **
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	// Check prefix matches
	if prefix != "" && !strings.HasPrefix(relPath, prefix+"/") {
		return false
	}

	// Get the part after the prefix
	rest := relPath
	if prefix != "" {
		rest = strings.TrimPrefix(relPath, prefix+"/")
	}

	// If no suffix, everything under prefix matches
	if suffix == "" {
		return true
	}

	// Check if any path segment satisfies the suffix glob
	segments := strings.Split(rest, "/")
	for i := range segments {
		candidate := strings.Join(segments[i:], "/")
		matched, _ := filepath.Match(suffix, candidate)
		if matched {
			return true
		}
	}
	return false
}

func isDir(pattern string) bool {
	// Heuristic: if pattern has no glob chars and no extension, treat as directory
	if strings.ContainsAny(pattern, "*?[") {
		return false
	}
	return filepath.Ext(pattern) == ""
}

// SyncBack copies files matching context patterns from the session dir
// back to the agent template dir. Returns the list of synced file paths
// (relative to the agent dir).
func SyncBack(sessionDir, agentDir string, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	var synced []string
	err := filepath.Walk(sessionDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip .claude directory
			if info.Name() == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(sessionDir, path)
		if err != nil {
			return err
		}

		// Map CLAUDE.md back to agent.md in the template
		dstRel := rel
		if rel == "CLAUDE.md" {
			dstRel = "agent.md"
		}

		if !MatchesAny(dstRel, patterns) {
			return nil
		}

		dst := filepath.Join(agentDir, dstRel)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := fileutil.CopyFile(path, dst); err != nil {
			return err
		}
		synced = append(synced, dstRel)
		return nil
	})
	return synced, err
}

// SyncFile copies a single file from session dir to agent dir if it matches
// any context pattern. Returns true if the file was synced.
func SyncFile(filePath, sessionDir, agentDir string, patterns []string) (bool, error) {
	if len(patterns) == 0 {
		return false, nil
	}

	rel, err := filepath.Rel(sessionDir, filePath)
	if err != nil {
		return false, nil
	}

	// Map CLAUDE.md back to agent.md in the template
	dstRel := rel
	if rel == "CLAUDE.md" {
		dstRel = "agent.md"
	}

	if !MatchesAny(dstRel, patterns) {
		return false, nil
	}

	dst := filepath.Join(agentDir, dstRel)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false, err
	}
	return true, fileutil.CopyFile(filePath, dst)
}

// HookSettings generates the .claude/settings.json content for hooks.
// When onEnd is non-empty, a SessionEnd hook with an agent-type handler is added
// so that Claude is prompted to persist context before the session closes.
func HookSettings(syncScriptPath string, onEnd string) ([]byte, error) {
	hooks := map[string]interface{}{
		"PostToolUse": []map[string]interface{}{
			{
				"matcher": "Edit|Write|MultiEdit",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": syncScriptPath,
					},
				},
			},
		},
	}

	if onEnd != "" {
		hooks["SessionEnd"] = []map[string]interface{}{
			{
				"hooks": []map[string]interface{}{
					{
						"type":   "agent",
						"prompt": onEnd,
					},
				},
			},
		}
	}

	settings := map[string]interface{}{
		"hooks": hooks,
	}
	return json.MarshalIndent(settings, "", "  ")
}

// MergePermissionHooks adds PreToolUse hooks for permission enforcement into
// an existing settings map. If settings is nil, creates a new one.
func MergePermissionHooks(settings map[string]interface{}, scriptPath string) map[string]interface{} {
	if settings == nil {
		settings = map[string]interface{}{}
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	preToolUse, _ := hooks["PreToolUse"].([]interface{})

	// Add the permission enforcement hook — it matches all tools
	preToolUse = append(preToolUse, map[string]interface{}{
		"hooks": []map[string]interface{}{
			{
				"type":    "command",
				"command": scriptPath,
			},
		},
	})

	hooks["PreToolUse"] = preToolUse
	settings["hooks"] = hooks
	return settings
}

// HookSettingsWithPermissions generates settings.json content that includes
// both existing hooks (sync, on_end) and permission enforcement hooks.
func HookSettingsWithPermissions(syncScriptPath string, onEnd string, permScriptPath string) ([]byte, error) {
	hooks := map[string]interface{}{}

	if syncScriptPath != "" {
		hooks["PostToolUse"] = []map[string]interface{}{
			{
				"matcher": "Edit|Write|MultiEdit",
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": syncScriptPath,
					},
				},
			},
		}
	}

	if onEnd != "" {
		hooks["SessionEnd"] = []map[string]interface{}{
			{
				"hooks": []map[string]interface{}{
					{
						"type":   "agent",
						"prompt": onEnd,
					},
				},
			},
		}
	}

	if permScriptPath != "" {
		hooks["PreToolUse"] = []map[string]interface{}{
			{
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": permScriptPath,
					},
				},
			},
		}
	}

	settings := map[string]interface{}{
		"hooks": hooks,
	}
	return json.MarshalIndent(settings, "", "  ")
}

// OnEndHookSettings generates a .claude/settings.json with only a SessionEnd hook.
// Used when context sync patterns are not configured but on_end is.
func OnEndHookSettings(onEnd string) ([]byte, error) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionEnd": []map[string]interface{}{
				{
					"hooks": []map[string]interface{}{
						{
							"type":   "agent",
							"prompt": onEnd,
						},
					},
				},
			},
		},
	}
	return json.MarshalIndent(settings, "", "  ")
}

// SyncScript generates a shell script that reads the PostToolUse hook payload
// from stdin and syncs matching files back to the agent directory.
func SyncScript(sessionDir, agentDir string, patterns []string) string {
	// Build pattern array for the shell script
	var patternArgs []string
	for _, p := range patterns {
		patternArgs = append(patternArgs, fmt.Sprintf("%q", p))
	}

	// The hook script delegates to `toc` itself for reliable pattern matching.
	// This avoids reimplementing glob logic in bash.
	return fmt.Sprintf(`#!/usr/bin/env bash
# Auto-generated by toc — syncs context files back to agent template.
# This script is called by Claude Code's PostToolUse hook.

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"file_path"[[:space:]]*:[[:space:]]*"//' | sed 's/"$//')

if [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Resolve to absolute path if relative
if [[ "$FILE_PATH" != /* ]]; then
  FILE_PATH="%s/$FILE_PATH"
fi

# Check file exists
if [ ! -f "$FILE_PATH" ]; then
  exit 0
fi

# Get path relative to session dir
REL_PATH="${FILE_PATH#%s/}"

# Skip if the path didn't change (file is outside session dir)
if [ "$REL_PATH" = "$FILE_PATH" ]; then
  exit 0
fi

# Map CLAUDE.md back to agent.md for the agent template
DST_REL="$REL_PATH"
if [ "$REL_PATH" = "CLAUDE.md" ]; then
  DST_REL="agent.md"
fi

# Check against each pattern and sync if matched
AGENT_DIR="%s"
PATTERNS=(%s)

for PATTERN in "${PATTERNS[@]}"; do
  # Directory pattern
  DIR_PATTERN="${PATTERN%%/}"
  if [ "$DIR_PATTERN" != "$PATTERN" ] || [ -d "%s/$DIR_PATTERN" ] 2>/dev/null; then
    if [[ "$DST_REL" == "$DIR_PATTERN"/* ]] || [ "$DST_REL" = "$DIR_PATTERN" ]; then
      mkdir -p "$(dirname "$AGENT_DIR/$DST_REL")"
      cp "$FILE_PATH" "$AGENT_DIR/$DST_REL"
      exit 0
    fi
  fi

  # Glob pattern — use bash built-in matching
  # shellcheck disable=SC2254
  case "$DST_REL" in
    $PATTERN)
      mkdir -p "$(dirname "$AGENT_DIR/$DST_REL")"
      cp "$FILE_PATH" "$AGENT_DIR/$DST_REL"
      exit 0
      ;;
  esac

  # Also match just the filename for non-path patterns
  BASENAME=$(basename "$DST_REL")
  if [[ "$PATTERN" != */* ]]; then
    # shellcheck disable=SC2254
    case "$BASENAME" in
      $PATTERN)
        mkdir -p "$(dirname "$AGENT_DIR/$DST_REL")"
        cp "$FILE_PATH" "$AGENT_DIR/$DST_REL"
        exit 0
        ;;
    esac
  fi
done
`, sessionDir, sessionDir, agentDir, strings.Join(patternArgs, " "), sessionDir)
}

// PermissionScript generates a shell script that enforces the agent's permission
// config as a Claude Code PreToolUse hook. It reads the tool invocation JSON from
// stdin and outputs a JSON decision: allow, block, or ask (via exit codes).
//
// Claude Code PreToolUse hook protocol:
//   - stdout JSON with {"decision": "block", "reason": "..."} to deny
//   - stdout JSON with {"decision": "allow"} to permit without prompting
//   - no output (or empty) to fall through to default behavior (ask user)
func PermissionScript(perms agent.Permissions, agentName string) string {
	// Build the tool→decision mapping for the script.
	// Read tools: Read, Glob, Grep
	// Write tools: Edit, Write, MultiEdit, NotebookEdit
	// Execute tools: Bash
	// Integration tools: matched by name prefix (e.g. "mcp__slack__")

	readDecision := string(perms.Filesystem.Read)
	writeDecision := string(perms.Filesystem.Write)
	execDecision := string(perms.Filesystem.Execute)

	// Build integration rules as bash case patterns
	var integrationCases strings.Builder
	for name, level := range perms.Integrations {
		integrationCases.WriteString(fmt.Sprintf("    mcp__%s__*)\n", name))
		switch level {
		case agent.PermOff:
			integrationCases.WriteString(fmt.Sprintf(`      echo '{"decision":"block","reason":"Agent \"%s\" does not have permission to use integration: %s"}'`+"\n", agentName, name))
			integrationCases.WriteString("      exit 0\n")
		case agent.PermAsk:
			integrationCases.WriteString("      # fall through — Claude Code will prompt the user\n")
			integrationCases.WriteString("      exit 0\n")
		default: // on
			integrationCases.WriteString(`      echo '{"decision":"allow"}'` + "\n")
			integrationCases.WriteString("      exit 0\n")
		}
		integrationCases.WriteString("      ;;\n")
	}

	script := fmt.Sprintf(`#!/usr/bin/env bash
# Auto-generated by toc — enforces agent permission config.
# This script is called by Claude Code's PreToolUse hook.
# Agent: %s

set -euo pipefail

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | grep -o '"tool_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"tool_name"[[:space:]]*:[[:space:]]*"//' | sed 's/"$//')

if [ -z "$TOOL_NAME" ]; then
  exit 0
fi

# --- Permission decisions ---

decide_read() {
  local LEVEL="%s"
  case "$LEVEL" in
    off)
      echo '{"decision":"block","reason":"Agent \"%s\" does not have filesystem read permission"}'
      ;;
    ask)
      # no output — fall through to Claude Code's default (prompt user)
      ;;
    *)
      echo '{"decision":"allow"}'
      ;;
  esac
}

decide_write() {
  local LEVEL="%s"
  case "$LEVEL" in
    off)
      echo '{"decision":"block","reason":"Agent \"%s\" does not have filesystem write permission"}'
      ;;
    ask)
      ;;
    *)
      echo '{"decision":"allow"}'
      ;;
  esac
}

decide_exec() {
  local LEVEL="%s"
  case "$LEVEL" in
    off)
      echo '{"decision":"block","reason":"Agent \"%s\" does not have shell execute permission"}'
      ;;
    ask)
      ;;
    *)
      echo '{"decision":"allow"}'
      ;;
  esac
}

# --- Tool routing ---

case "$TOOL_NAME" in
  Read|Glob|Grep)
    decide_read
    ;;
  Edit|Write|MultiEdit|NotebookEdit)
    decide_write
    ;;
  Bash)
    decide_exec
    ;;
%s  *)
    # Unknown tool — allow by default
    echo '{"decision":"allow"}'
    ;;
esac
`, agentName,
		readDecision, agentName,
		writeDecision, agentName,
		execDecision, agentName,
		integrationCases.String())

	return script
}

