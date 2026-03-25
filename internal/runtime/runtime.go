package runtime

import (
	"fmt"
	"os"

	"github.com/tiny-oc/toc/internal/agent"
)

// Context holds the runtime context for an agent session, resolved from env vars.
type Context struct {
	Workspace string
	Agent     string
	SessionID string
}

// FromEnv reads the runtime context from environment variables.
// Returns an error if not running inside a toc session.
func FromEnv() (*Context, error) {
	workspace := os.Getenv("TOC_WORKSPACE")
	agentName := os.Getenv("TOC_AGENT")
	sessionID := os.Getenv("TOC_SESSION_ID")

	if workspace == "" || agentName == "" || sessionID == "" {
		return nil, fmt.Errorf("not running inside a toc session — toc runtime commands can only be used by agents during a session")
	}

	return &Context{
		Workspace: workspace,
		Agent:     agentName,
		SessionID: sessionID,
	}, nil
}

// LoadAgentConfig loads the agent config for this runtime context.
func (ctx *Context) LoadAgentConfig() (*agent.AgentConfig, error) {
	// Override the working directory to the workspace so agent.Load works
	orig, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(ctx.Workspace); err != nil {
		return nil, fmt.Errorf("failed to access workspace: %w", err)
	}
	defer os.Chdir(orig)

	return agent.Load(ctx.Agent)
}

// LoadSessionConfig loads the resolved per-session config for this runtime context.
func (ctx *Context) LoadSessionConfig() (*SessionConfig, error) {
	return LoadSessionConfigInWorkspace(ctx.Workspace, ctx.SessionID)
}

// LoadTargetAgent loads a target agent config from the workspace.
func (ctx *Context) LoadTargetAgent(name string) (*agent.AgentConfig, error) {
	orig, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(ctx.Workspace); err != nil {
		return nil, fmt.Errorf("failed to access workspace: %w", err)
	}
	defer os.Chdir(orig)

	return agent.Load(name)
}

// ListAgents lists all agents in the workspace.
func (ctx *Context) ListAgents() ([]agent.AgentConfig, error) {
	orig, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(ctx.Workspace); err != nil {
		return nil, fmt.Errorf("failed to access workspace: %w", err)
	}
	defer os.Chdir(orig)

	return agent.List()
}
