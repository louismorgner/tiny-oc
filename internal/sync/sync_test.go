package sync

import "testing"

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
