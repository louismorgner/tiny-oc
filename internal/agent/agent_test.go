package agent

import (
	"strings"
	"testing"
)

func TestEffectivePermissions_Defaults(t *testing.T) {
	cfg := &AgentConfig{Name: "test", Runtime: "claude-code", Model: "sonnet"}
	perms := cfg.EffectivePermissions()

	if perms.Filesystem.Read != PermOn {
		t.Errorf("expected default read=on, got %s", perms.Filesystem.Read)
	}
	if perms.Filesystem.Write != PermOn {
		t.Errorf("expected default write=on, got %s", perms.Filesystem.Write)
	}
	if perms.Filesystem.Execute != PermOn {
		t.Errorf("expected default execute=on, got %s", perms.Filesystem.Execute)
	}
}

func TestEffectivePermissions_ExplicitBlock(t *testing.T) {
	cfg := &AgentConfig{
		Name: "test", Runtime: "claude-code", Model: "sonnet",
		Perms: &Permissions{
			Filesystem: FilesystemPermissions{
				Read:    PermOn,
				Write:   PermAsk,
				Execute: PermOff,
			},
		},
	}
	perms := cfg.EffectivePermissions()

	if perms.Filesystem.Read != PermOn {
		t.Errorf("expected read=on, got %s", perms.Filesystem.Read)
	}
	if perms.Filesystem.Write != PermAsk {
		t.Errorf("expected write=ask, got %s", perms.Filesystem.Write)
	}
	if perms.Filesystem.Execute != PermOff {
		t.Errorf("expected execute=off, got %s", perms.Filesystem.Execute)
	}
}

func TestEffectivePermissions_SubAgents(t *testing.T) {
	cfg := &AgentConfig{
		Name: "test", Runtime: "claude-code", Model: "sonnet",
		Perms: &Permissions{
			SubAgents: map[string]PermissionLevel{
				"cto": PermAsk,
				"*":   PermOff,
			},
		},
	}
	perms := cfg.EffectivePermissions()

	if perms.SubAgents["cto"] != PermAsk {
		t.Errorf("expected sub-agent cto=ask, got %s", perms.SubAgents["cto"])
	}
	if perms.SubAgents["*"] != PermOff {
		t.Errorf("expected sub-agent *=off, got %s", perms.SubAgents["*"])
	}
}

func TestCanSpawn(t *testing.T) {
	tests := []struct {
		name   string
		cfg    AgentConfig
		target string
		want   bool
	}{
		{
			name: "explicit on",
			cfg: AgentConfig{Perms: &Permissions{
				SubAgents: map[string]PermissionLevel{"cto": PermOn},
			}},
			target: "cto",
			want:   true,
		},
		{
			name: "explicit ask allows spawn",
			cfg: AgentConfig{Perms: &Permissions{
				SubAgents: map[string]PermissionLevel{"cto": PermAsk},
			}},
			target: "cto",
			want:   true,
		},
		{
			name: "explicit off blocks",
			cfg: AgentConfig{Perms: &Permissions{
				SubAgents: map[string]PermissionLevel{"cto": PermOff},
			}},
			target: "cto",
			want:   false,
		},
		{
			name: "wildcard off blocks",
			cfg: AgentConfig{Perms: &Permissions{
				SubAgents: map[string]PermissionLevel{"*": PermOff},
			}},
			target: "anything",
			want:   false,
		},
		{
			name: "specific overrides wildcard",
			cfg: AgentConfig{Perms: &Permissions{
				SubAgents: map[string]PermissionLevel{
					"*":   PermOff,
					"cto": PermOn,
				},
			}},
			target: "cto",
			want:   true,
		},
		{
			name:   "no permissions at all",
			cfg:    AgentConfig{},
			target: "anything",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.CanSpawn(tt.target)
			if got != tt.want {
				t.Errorf("CanSpawn(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestValidate_PermissionLevels(t *testing.T) {
	cfg := &AgentConfig{
		Name: "test", Runtime: "claude-code", Model: "sonnet",
		Perms: &Permissions{
			Filesystem: FilesystemPermissions{
				Read: "invalid",
			},
		},
	}
	problems := cfg.Validate()
	if len(problems) == 0 {
		t.Error("expected validation error for invalid permission level")
	}
}

func TestValidate_NativeCustomModelRequiresOverride(t *testing.T) {
	cfg := &AgentConfig{
		Name:    "test",
		Runtime: "toc-native",
		Model:   "meta-llama/unknown",
	}
	problems := cfg.Validate()
	if len(problems) == 0 {
		t.Fatal("expected validation error for unsupported custom native model")
	}
}

func TestValidate_NativeCustomModelAllowedWithOverride(t *testing.T) {
	cfg := &AgentConfig{
		Name:                   "test",
		Runtime:                "toc-native",
		Model:                  "meta-llama/unknown",
		AllowCustomNativeModel: true,
	}
	problems := cfg.Validate()
	if len(problems) != 0 {
		t.Fatalf("expected override to allow custom native model, got %v", problems)
	}
}

func TestValidate_ClaudeCodeDefaultModelAllowed(t *testing.T) {
	cfg := &AgentConfig{
		Name:    "test",
		Runtime: "claude-code",
		Model:   "default",
	}
	problems := cfg.Validate()
	if len(problems) != 0 {
		t.Fatalf("expected default model to be valid for claude-code, got %v", problems)
	}
}

func TestValidate_SmallModelRequiresNativeOverride(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Runtime:    "toc-native",
		Model:      "openai/gpt-4o-mini",
		SmallModel: "meta-llama/unknown",
	}
	problems := cfg.Validate()
	if len(problems) == 0 {
		t.Fatal("expected validation error for unsupported custom small_model")
	}
	if got := problems[0]; !strings.Contains(got, "invalid small_model:") {
		t.Fatalf("expected small_model validation error, got %v", problems)
	}
}

func TestValidate_SmallModelAllowedWithNativeOverride(t *testing.T) {
	cfg := &AgentConfig{
		Name:                   "test",
		Runtime:                "toc-native",
		Model:                  "openai/gpt-4o-mini",
		SmallModel:             "meta-llama/unknown",
		AllowCustomNativeModel: true,
	}
	problems := cfg.Validate()
	if len(problems) != 0 {
		t.Fatalf("expected override to allow custom small_model, got %v", problems)
	}
}

func TestSubAgentPermission(t *testing.T) {
	cfg := &AgentConfig{
		Perms: &Permissions{
			SubAgents: map[string]PermissionLevel{
				"cto": PermAsk,
				"*":   PermOff,
			},
		},
	}
	if got := cfg.SubAgentPermission("cto"); got != PermAsk {
		t.Errorf("SubAgentPermission(cto) = %s, want ask", got)
	}
	if got := cfg.SubAgentPermission("other"); got != PermOff {
		t.Errorf("SubAgentPermission(other) = %s, want off", got)
	}
	if got := cfg.SubAgentPermission("unknown"); got != PermOff {
		t.Errorf("SubAgentPermission(unknown) = %s, want off", got)
	}
}
