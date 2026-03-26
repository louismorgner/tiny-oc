package runtime

import (
	"strings"
	"testing"

	"github.com/tiny-oc/toc/internal/agent"
)

func TestBuildClaudePermissionHookScript_ContainsDecisions(t *testing.T) {
	perms := agent.Permissions{
		Filesystem: agent.FilesystemPermissions{
			Read:    agent.PermOn,
			Write:   agent.PermAsk,
			Execute: agent.PermOff,
		},
		Integrations: map[string]agent.IntegrationPermissions{
			"slack": {
				{Mode: agent.PermOn, Capability: "send_message:*"},
			},
			"github": {
				{Mode: agent.PermOn, Capability: "issues.read:*"},
			},
		},
	}

	script := buildClaudePermissionHookScript(perms, "test-agent")

	if !strings.Contains(script, `local LEVEL="on"`) {
		t.Error("expected read level 'on' in script")
	}
	if !strings.Contains(script, `local LEVEL="ask"`) {
		t.Error("expected write level 'ask' in script")
	}
	if !strings.Contains(script, `local LEVEL="off"`) {
		t.Error("expected execute level 'off' in script")
	}
	if strings.Contains(script, "mcp__slack__") {
		t.Error("integration patterns should not be in hook script")
	}
	if !strings.HasPrefix(script, "#!/usr/bin/env bash") {
		t.Error("expected bash shebang")
	}
}

func TestBuildClaudeHookSettings(t *testing.T) {
	data, err := buildClaudeHookSettings("/path/sync.sh", "persist context", "/path/perms.sh")
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

func TestBuildClaudeHookSettings_PermissionsOnly(t *testing.T) {
	data, err := buildClaudeHookSettings("", "", "/path/perms.sh")
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
