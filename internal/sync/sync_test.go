package sync

import (
	"strings"
	"testing"

	"github.com/tiny-oc/toc/internal/agent"
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

func TestPermissionScript_ContainsDecisions(t *testing.T) {
	perms := agent.Permissions{
		Filesystem: agent.FilesystemPermissions{
			Read:    agent.PermOn,
			Write:   agent.PermAsk,
			Execute: agent.PermOff,
		},
		Integrations: map[string][]string{
			"slack":  {"send_message:*"},
			"github": {"issues.read:*"},
		},
	}

	script := PermissionScript(perms, "test-agent")

	// Check filesystem decisions are embedded
	if !strings.Contains(script, `local LEVEL="on"`) {
		t.Error("expected read level 'on' in script")
	}
	if !strings.Contains(script, `local LEVEL="ask"`) {
		t.Error("expected write level 'ask' in script")
	}
	if !strings.Contains(script, `local LEVEL="off"`) {
		t.Error("expected execute level 'off' in script")
	}

	// Integration permissions are now enforced by the gateway, not hook scripts.
	// Verify integration patterns are NOT in the script.
	if strings.Contains(script, "mcp__slack__") {
		t.Error("integration patterns should not be in hook script (enforced by gateway)")
	}

	// Check it's valid bash (starts with shebang)
	if !strings.HasPrefix(script, "#!/usr/bin/env bash") {
		t.Error("expected bash shebang")
	}
}

func TestHookSettingsWithPermissions(t *testing.T) {
	data, err := HookSettingsWithPermissions("/path/sync.sh", "persist context", "/path/perms.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	if !strings.Contains(s, "PreToolUse") {
		t.Error("expected PreToolUse hook in settings")
	}
	if !strings.Contains(s, "PostToolUse") {
		t.Error("expected PostToolUse hook in settings")
	}
	if !strings.Contains(s, "SessionEnd") {
		t.Error("expected SessionEnd hook in settings")
	}
	if !strings.Contains(s, "/path/perms.sh") {
		t.Error("expected permission script path in settings")
	}
}

func TestHookSettingsWithPermissions_PermissionsOnly(t *testing.T) {
	data, err := HookSettingsWithPermissions("", "", "/path/perms.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	if !strings.Contains(s, "PreToolUse") {
		t.Error("expected PreToolUse hook in settings")
	}
	if strings.Contains(s, "PostToolUse") {
		t.Error("did not expect PostToolUse hook when no sync script")
	}
}
