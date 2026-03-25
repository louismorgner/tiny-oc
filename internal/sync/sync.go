package sync

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tiny-oc/toc/internal/fileutil"
)

type Options struct {
	PathMapper func(string) string
	SkipDirs   map[string]bool
}

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

func mapPath(rel string, opts Options) string {
	if opts.PathMapper != nil {
		return opts.PathMapper(rel)
	}
	return rel
}

// SyncBack copies files matching context patterns from the session dir
// back to the agent template dir. Returns the list of synced file paths
// (relative to the agent dir).
func SyncBack(sessionDir, agentDir string, patterns []string) ([]string, error) {
	return SyncBackWithOptions(sessionDir, agentDir, patterns, Options{})
}

func SyncBackWithOptions(sessionDir, agentDir string, patterns []string, opts Options) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	var synced []string
	err := filepath.Walk(sessionDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if opts.SkipDirs != nil && opts.SkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(sessionDir, path)
		if err != nil {
			return err
		}

		dstRel := mapPath(rel, opts)

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
	return SyncFileWithOptions(filePath, sessionDir, agentDir, patterns, Options{})
}

func SyncFileWithOptions(filePath, sessionDir, agentDir string, patterns []string, opts Options) (bool, error) {
	if len(patterns) == 0 {
		return false, nil
	}

	rel, err := filepath.Rel(sessionDir, filePath)
	if err != nil {
		return false, nil
	}

	dstRel := mapPath(rel, opts)

	if !MatchesAny(dstRel, patterns) {
		return false, nil
	}

	dst := filepath.Join(agentDir, dstRel)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false, err
	}
	return true, fileutil.CopyFile(filePath, dst)
}
