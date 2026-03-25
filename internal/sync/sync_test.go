package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		relPath  string
		patterns []string
		want     bool
	}{
		// Glob patterns
		{"context/notes.md", []string{"context/*.md"}, true},
		{"context/deep/notes.md", []string{"context/*.md"}, false},
		{"readme.txt", []string{"context/*.md"}, false},

		// Directory patterns
		{"docs/api.md", []string{"docs/"}, true},
		{"docs/sub/file.txt", []string{"docs/"}, true},
		{"docs", []string{"docs/"}, true},
		{"other/file.txt", []string{"docs/"}, false},

		// Bare directory name (no trailing slash)
		{"docs/api.md", []string{"docs"}, true},
		{"docs/sub/file.txt", []string{"docs"}, true},

		// Doublestar
		{"context/a/b/c.md", []string{"context/**/*.md"}, true},
		{"context/notes.md", []string{"context/**/*.md"}, true},
		{"other/notes.md", []string{"context/**/*.md"}, false},

		// Simple filename patterns
		{"foo/bar/notes.txt", []string{"notes.txt"}, true},
		{"notes.txt", []string{"notes.txt"}, true},
		{"other.txt", []string{"notes.txt"}, false},

		// Wildcard filename
		{"foo/bar.md", []string{"*.md"}, true},
		{"deep/nested/file.md", []string{"*.md"}, true},
		{"file.txt", []string{"*.md"}, false},

		// Multiple patterns
		{"docs/api.md", []string{"context/*.md", "docs/"}, true},
		{"context/notes.md", []string{"context/*.md", "docs/"}, true},
		{"other.txt", []string{"context/*.md", "docs/"}, false},

		// Empty patterns
		{"anything.txt", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.relPath+"_"+joinPatterns(tt.patterns), func(t *testing.T) {
			got := MatchesAny(tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("MatchesAny(%q, %v) = %v, want %v", tt.relPath, tt.patterns, got, tt.want)
			}
		})
	}
}

func joinPatterns(p []string) string {
	s := ""
	for i, v := range p {
		if i > 0 {
			s += ","
		}
		s += v
	}
	return s
}

func TestSyncBackWithOptions_MapsInstructionFileAndSkipsDirs(t *testing.T) {
	sessionDir := t.TempDir()
	agentDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(sessionDir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, ".hidden", "skip.md"), []byte("skip"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "CLAUDE.md"), []byte("instruction"), 0644); err != nil {
		t.Fatal(err)
	}

	synced, err := SyncBackWithOptions(sessionDir, agentDir, []string{"agent.md"}, Options{
		PathMapper: func(rel string) string {
			if rel == "CLAUDE.md" {
				return "agent.md"
			}
			return rel
		},
		SkipDirs: map[string]bool{".hidden": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != "agent.md" {
		t.Fatalf("synced = %#v", synced)
	}
	data, err := os.ReadFile(filepath.Join(agentDir, "agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "instruction" {
		t.Fatalf("agent.md = %q", string(data))
	}
}
