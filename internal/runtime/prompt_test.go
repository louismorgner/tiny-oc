package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
)

func TestComposePrompt_SubAgentInstructions_ClaudeCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.md"), []byte("# test\n\n{{.SubAgentInstructions}}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Agent:   "test-agent",
		Runtime: runtimeinfo.DefaultRuntime,
		Model:   "sonnet",
		Permissions: agent.Permissions{
			SubAgents: map[string]agent.PermissionLevel{"*": agent.PermOn},
		},
	}

	content, err := ComposePrompt(dir, cfg, "test-session")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(content, "toc runtime spawn") {
		t.Error("expected claude-code sub-agent instructions with 'toc runtime spawn'")
	}
	if strings.Contains(content, "SubAgent tool") {
		t.Error("should not contain native SubAgent tool instructions")
	}
}

func TestComposePrompt_SubAgentInstructions_Native(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.md"), []byte("# test\n\n{{.SubAgentInstructions}}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Agent:   "test-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "anthropic/claude-sonnet-4",
		Permissions: agent.Permissions{
			SubAgents: map[string]agent.PermissionLevel{"cto": agent.PermOn},
		},
	}

	content, err := ComposePrompt(dir, cfg, "test-session")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(content, "SubAgent tool") {
		t.Error("expected native sub-agent instructions with 'SubAgent tool'")
	}
	if strings.Contains(content, "toc runtime spawn") {
		t.Error("should not contain CLI sub-agent instructions")
	}
}

func TestComposePrompt_SubAgentInstructions_NoPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.md"), []byte("# test\n\n{{.SubAgentInstructions}}\n\nend"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Agent:   "test-agent",
		Runtime: runtimeinfo.DefaultRuntime,
		Model:   "sonnet",
		// No sub-agent permissions
	}

	content, err := ComposePrompt(dir, cfg, "test-session")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(content, "Sub-agents") {
		t.Error("should not contain sub-agent instructions when no permissions are set")
	}
	if !strings.Contains(content, "end") {
		t.Error("rest of content should still be present")
	}
}

func TestComposePrompt_SubAgentInstructions_AllOff(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.md"), []byte("{{.SubAgentInstructions}}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Agent:   "test-agent",
		Runtime: runtimeinfo.DefaultRuntime,
		Model:   "sonnet",
		Permissions: agent.Permissions{
			SubAgents: map[string]agent.PermissionLevel{"cto": agent.PermOff},
		},
	}

	content, err := ComposePrompt(dir, cfg, "test-session")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(content, "Sub-agents") {
		t.Error("should not contain sub-agent instructions when all permissions are off")
	}
}
