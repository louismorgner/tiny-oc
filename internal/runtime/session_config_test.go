package runtime

import (
	"os"
	"testing"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestResolveSessionConfig(t *testing.T) {
	cfg := ResolveSessionConfig(&agent.AgentConfig{
		Name:                   "native-agent",
		Runtime:                runtimeinfo.NativeRuntime,
		Model:                  "openai/gpt-4o-mini",
		AllowCustomNativeModel: true,
		Context:                []string{"context/*.md"},
		Skills:                 []string{"code-review"},
		Compose:                []string{"extra.md"},
		OnEnd:                  "persist summary",
		Perms: &agent.Permissions{
			Filesystem: agent.FilesystemPermissions{
				Read:    agent.PermOn,
				Write:   agent.PermOff,
				Execute: agent.PermOff,
			},
		},
	})

	if cfg.Agent != "native-agent" || cfg.Runtime != runtimeinfo.NativeRuntime {
		t.Fatalf("resolved session config = %#v", cfg)
	}
	if cfg.LLM.Provider != "openrouter" {
		t.Fatalf("LLM provider = %q", cfg.LLM.Provider)
	}
	if len(cfg.RuntimeConfig.EnabledTools) == 0 {
		t.Fatalf("expected native tools in runtime config: %#v", cfg.RuntimeConfig)
	}
	if cfg.Permissions.Filesystem.Write != agent.PermOff {
		t.Fatalf("expected resolved permissions, got %#v", cfg.Permissions)
	}
	if !cfg.AllowCustomNativeModel {
		t.Fatalf("expected AllowCustomNativeModel to propagate, got %#v", cfg)
	}
	if cfg.RuntimeConfig.CompactionTriggerChars != 0 {
		t.Fatalf("CompactionTriggerChars should no longer be set by default (token budgets are primary), got %d", cfg.RuntimeConfig.CompactionTriggerChars)
	}
	if cfg.RuntimeConfig.CompactionKeepRecent != 12 {
		t.Fatalf("CompactionKeepRecent = %d, want 12", cfg.RuntimeConfig.CompactionKeepRecent)
	}
}

func TestSaveAndLoadSessionConfig(t *testing.T) {
	sess := &session.Session{ID: "sess-config", MetadataDir: t.TempDir()}
	cfg := &SessionConfig{
		Agent:   "tester",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		LLM:     SessionLLMConfig{Provider: "openrouter"},
	}

	if err := SaveSessionConfig(sess, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSessionConfig(sess)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != SessionConfigVersion {
		t.Fatalf("Version = %d, want %d", loaded.Version, SessionConfigVersion)
	}
	if loaded.Model != cfg.Model || loaded.LLM.Provider != "openrouter" {
		t.Fatalf("loaded session config = %#v", loaded)
	}
}

func TestResolveSessionConfig_MaxIterations_Default(t *testing.T) {
	cfg := ResolveSessionConfig(&agent.AgentConfig{
		Name:    "test-agent",
		Runtime: runtimeinfo.NativeRuntime,
		Model:   "openai/gpt-4o-mini",
		AllowCustomNativeModel: true,
	})
	if cfg.RuntimeConfig.MaxIterations != defaultMaxIterations {
		t.Fatalf("MaxIterations = %d, want %d", cfg.RuntimeConfig.MaxIterations, defaultMaxIterations)
	}
}

func TestResolveSessionConfig_MaxIterations_AgentYAML(t *testing.T) {
	cfg := ResolveSessionConfig(&agent.AgentConfig{
		Name:          "test-agent",
		Runtime:       runtimeinfo.NativeRuntime,
		Model:         "openai/gpt-4o-mini",
		MaxIterations: 50,
		AllowCustomNativeModel: true,
	})
	if cfg.RuntimeConfig.MaxIterations != 50 {
		t.Fatalf("MaxIterations = %d, want 50", cfg.RuntimeConfig.MaxIterations)
	}
}

func TestResolveSessionConfig_MaxIterations_EnvVar(t *testing.T) {
	t.Setenv("TOC_MAX_ITERATIONS", "100")
	cfg := ResolveSessionConfig(&agent.AgentConfig{
		Name:          "test-agent",
		Runtime:       runtimeinfo.NativeRuntime,
		Model:         "openai/gpt-4o-mini",
		MaxIterations: 50,
		AllowCustomNativeModel: true,
	})
	if cfg.RuntimeConfig.MaxIterations != 100 {
		t.Fatalf("MaxIterations = %d, want 100 (env should override agent yaml)", cfg.RuntimeConfig.MaxIterations)
	}
}

func TestResolveSessionConfig_MaxIterations_CLIOverride(t *testing.T) {
	t.Setenv("TOC_MAX_ITERATIONS", "100")
	cfg := ResolveSessionConfig(&agent.AgentConfig{
		Name:          "test-agent",
		Runtime:       runtimeinfo.NativeRuntime,
		Model:         "openai/gpt-4o-mini",
		MaxIterations: 50,
		AllowCustomNativeModel: true,
	}, ResolveOptions{MaxIterationsOverride: 200})
	if cfg.RuntimeConfig.MaxIterations != 200 {
		t.Fatalf("MaxIterations = %d, want 200 (CLI should override env and agent yaml)", cfg.RuntimeConfig.MaxIterations)
	}
}

func TestResolveSessionConfig_MaxIterations_InvalidEnvIgnored(t *testing.T) {
	t.Setenv("TOC_MAX_ITERATIONS", "notanumber")
	cfg := ResolveSessionConfig(&agent.AgentConfig{
		Name:          "test-agent",
		Runtime:       runtimeinfo.NativeRuntime,
		Model:         "openai/gpt-4o-mini",
		MaxIterations: 50,
		AllowCustomNativeModel: true,
	})
	if cfg.RuntimeConfig.MaxIterations != 50 {
		t.Fatalf("MaxIterations = %d, want 50 (invalid env should be ignored)", cfg.RuntimeConfig.MaxIterations)
	}
}

func TestLoadSessionConfigInWorkspace_Missing(t *testing.T) {
	_, err := LoadSessionConfigInWorkspace(t.TempDir(), "missing")
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist, got %v", err)
	}
}
