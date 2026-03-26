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

func TestLoadSessionConfigInWorkspace_Missing(t *testing.T) {
	_, err := LoadSessionConfigInWorkspace(t.TempDir(), "missing")
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist, got %v", err)
	}
}
