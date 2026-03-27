package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

const SessionConfigVersion = 1

type SessionLLMConfig struct {
	Provider string `json:"provider,omitempty"`
}

type SessionRuntimeOptions struct {
	EnabledTools              []string `json:"enabled_tools,omitempty"`
	MaxIterations             int      `json:"max_iterations,omitempty"`
	CompactionTriggerChars    int      `json:"compaction_trigger_chars,omitempty"`
	CompactionKeepRecent      int      `json:"compaction_keep_recent,omitempty"`
	CompactionMaxSummaryChars int      `json:"compaction_max_summary_chars,omitempty"`
}

// SessionConfig is the resolved, toc-owned session contract written at spawn
// time. Sessions resume from this config rather than re-reading oc-agent.yaml.
type SessionConfig struct {
	Version                int                   `json:"version"`
	Agent                  string                `json:"agent"`
	Runtime                string                `json:"runtime"`
	Model                  string                `json:"model"`
	AllowCustomNativeModel bool                  `json:"allow_custom_native_model,omitempty"`
	Description            string                `json:"description,omitempty"`
	Context                []string              `json:"context,omitempty"`
	Skills                 []string              `json:"skills,omitempty"`
	Compose                []string              `json:"compose,omitempty"`
	OnEnd                  string                `json:"on_end,omitempty"`
	Permissions            agent.Permissions     `json:"permissions,omitempty"`
	RuntimeConfig          SessionRuntimeOptions `json:"runtime_config,omitempty"`
	LLM                    SessionLLMConfig      `json:"llm,omitempty"`
}

// ResolveOptions contains optional overrides for session config resolution.
type ResolveOptions struct {
	MaxIterationsOverride int // CLI flag override; 0 means not set
}

func ResolveSessionConfig(cfg *agent.AgentConfig, opts ...ResolveOptions) *SessionConfig {
	if cfg == nil {
		return nil
	}

	var resolveOpts ResolveOptions
	if len(opts) > 0 {
		resolveOpts = opts[0]
	}

	sessionCfg := &SessionConfig{
		Version:                SessionConfigVersion,
		Agent:                  cfg.Name,
		Runtime:                cfg.Runtime,
		Model:                  cfg.Model,
		AllowCustomNativeModel: cfg.AllowCustomNativeModel,
		Description:            cfg.Description,
		Context:                append([]string(nil), cfg.Context...),
		Skills:                 append([]string(nil), cfg.Skills...),
		Compose:                append([]string(nil), cfg.Compose...),
		OnEnd:                  cfg.OnEnd,
		Permissions:            cfg.EffectivePermissions(),
		LLM: SessionLLMConfig{
			Provider: resolvedLLMProvider(cfg.Runtime),
		},
	}
	if cfg.Runtime == runtimeinfo.NativeRuntime {
		sessionCfg.RuntimeConfig.EnabledTools = NativeToolNames()
		// Priority: CLI flag > env var > oc-agent.yaml > hardcoded default
		sessionCfg.RuntimeConfig.MaxIterations = defaultMaxIterations
		if cfg.MaxIterations > 0 {
			sessionCfg.RuntimeConfig.MaxIterations = cfg.MaxIterations
		}
		if v, err := strconv.Atoi(os.Getenv("TOC_MAX_ITERATIONS")); err == nil && v > 0 {
			sessionCfg.RuntimeConfig.MaxIterations = v
		}
		if resolveOpts.MaxIterationsOverride > 0 {
			sessionCfg.RuntimeConfig.MaxIterations = resolveOpts.MaxIterationsOverride
		}
		sessionCfg.RuntimeConfig.CompactionTriggerChars = 800000
		sessionCfg.RuntimeConfig.CompactionKeepRecent = 12
		sessionCfg.RuntimeConfig.CompactionMaxSummaryChars = 6000
	}
	return sessionCfg
}

func ValidateSessionConfig(cfg *SessionConfig) error {
	if cfg == nil {
		return fmt.Errorf("session config is nil")
	}
	if err := runtimeinfo.ValidateModelSelection(cfg.Runtime, cfg.Model, cfg.AllowCustomNativeModel); err != nil {
		return err
	}
	for name, grants := range cfg.Permissions.Integrations {
		def, err := integration.LoadFromRegistry(name)
		if err != nil {
			return fmt.Errorf("invalid integration '%s': %w", name, err)
		}
		if err := integration.ValidatePermissionsAgainstDefinition(grants, def); err != nil {
			return fmt.Errorf("invalid permissions for integration '%s': %w", name, err)
		}
	}
	return nil
}

func resolvedLLMProvider(runtimeName string) string {
	switch runtimeName {
	case runtimeinfo.NativeRuntime:
		return "openrouter"
	case DefaultRuntime:
		return "claude-code"
	default:
		return ""
	}
}

func SessionConfigPath(sess *session.Session) string {
	if dir := sess.MetadataDirPath(); dir != "" {
		return filepath.Join(dir, "session.json")
	}
	return ""
}

func SessionConfigPathInWorkspace(workspace, sessionID string) string {
	return filepath.Join(MetadataDir(workspace, sessionID), "session.json")
}

func SaveSessionConfig(sess *session.Session, cfg *SessionConfig) error {
	path := SessionConfigPath(sess)
	if path == "" {
		return fmt.Errorf("session '%s' has no metadata directory for session config", sess.ID)
	}
	return saveSessionConfigToPath(path, cfg)
}

func SaveSessionConfigInWorkspace(workspace, sessionID string, cfg *SessionConfig) error {
	return saveSessionConfigToPath(SessionConfigPathInWorkspace(workspace, sessionID), cfg)
}

func LoadSessionConfig(sess *session.Session) (*SessionConfig, error) {
	path := SessionConfigPath(sess)
	if path == "" {
		return nil, fmt.Errorf("session '%s' has no metadata directory for session config", sess.ID)
	}
	return loadSessionConfigFromPath(path)
}

func LoadSessionConfigInWorkspace(workspace, sessionID string) (*SessionConfig, error) {
	return loadSessionConfigFromPath(SessionConfigPathInWorkspace(workspace, sessionID))
}

func loadSessionConfigFromPath(path string) (*SessionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg SessionConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveSessionConfigToPath(path string, cfg *SessionConfig) error {
	if cfg == nil {
		return fmt.Errorf("session config is nil")
	}
	if cfg.Version == 0 {
		cfg.Version = SessionConfigVersion
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
