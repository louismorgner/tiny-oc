package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/session"
)

func LoadPermissionManifest(sess *session.Session) (*integration.PermissionManifest, error) {
	path := filepath.Join(sess.MetadataDirPath(), "permissions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest integration.PermissionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func LoadPermissionManifestInWorkspace(workspace, sessionID string) (*integration.PermissionManifest, error) {
	sess := &session.Session{
		ID:          sessionID,
		MetadataDir: MetadataDir(workspace, sessionID),
	}
	return LoadPermissionManifest(sess)
}

// FilesystemPermissionLevel returns the permission level for the given filesystem
// operation kind. A nil manifest returns PermOn (the default), matching the
// permissive defaults from AgentConfig.EffectivePermissions().
func FilesystemPermissionLevel(manifest *integration.PermissionManifest, kind string) agent.PermissionLevel {
	if manifest == nil {
		return agent.PermOn
	}

	switch kind {
	case "read":
		if manifest.Filesystem.Read == "" {
			return agent.PermOn
		}
		return manifest.Filesystem.Read
	case "write":
		if manifest.Filesystem.Write == "" {
			return agent.PermOn
		}
		return manifest.Filesystem.Write
	case "execute":
		if manifest.Filesystem.Execute == "" {
			return agent.PermOn
		}
		return manifest.Filesystem.Execute
	default:
		return agent.PermOff
	}
}

// CanSpawnFromManifest checks whether the manifest allows spawning the named
// sub-agent. A nil manifest returns false (the default), matching the deny-by-
// default sub-agent policy from AgentConfig.EffectivePermissions() which
// produces an empty SubAgents map. This is intentionally different from
// FilesystemPermissionLevel's nil→allow behavior: filesystem defaults to
// permissive, sub-agent spawning defaults to restrictive.
func CanSpawnFromManifest(manifest *integration.PermissionManifest, target string) bool {
	if manifest == nil {
		return false
	}
	if level, ok := manifest.SubAgents[target]; ok {
		return level != agent.PermOff
	}
	if level, ok := manifest.SubAgents["*"]; ok {
		return level != agent.PermOff
	}
	return false
}

func ValidateFilesystemPermission(manifest *integration.PermissionManifest, kind, agentName string) error {
	level := FilesystemPermissionLevel(manifest, kind)
	switch level {
	case agent.PermOn:
		return nil
	case agent.PermAsk:
		return fmt.Errorf("permission denied: agent '%s' requires approval for filesystem %s access", agentName, kind)
	case agent.PermOff:
		return fmt.Errorf("permission denied: agent '%s' does not have filesystem %s access", agentName, kind)
	default:
		return fmt.Errorf("permission denied: agent '%s' has invalid filesystem %s permission", agentName, kind)
	}
}
