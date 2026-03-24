package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
// back to the agent template dir.
func SyncBack(sessionDir, agentDir string, patterns []string) (int, error) {
	if len(patterns) == 0 {
		return 0, nil
	}

	count := 0
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
		if err := copyFile(path, dst); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
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
	return true, copyFile(filePath, dst)
}

// HookSettings generates the .claude/settings.json content for the PostToolUse hook.
func HookSettings(syncScriptPath string) ([]byte, error) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
