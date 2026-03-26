package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// IntegrationPermissionGrant is one integration permission entry such as
// "post:#eng" or { ask: "post:*" }.
type IntegrationPermissionGrant struct {
	Mode       PermissionLevel `json:"mode" yaml:"mode,omitempty"`
	Capability string          `json:"capability" yaml:"capability,omitempty"`
}

// IntegrationPermissions accepts either a scalar string or a sequence of grant entries.
type IntegrationPermissions []IntegrationPermissionGrant

func (g IntegrationPermissionGrant) String() string {
	mode := g.Mode
	if mode == "" {
		mode = PermOn
	}
	return fmt.Sprintf("%s: %s", mode, g.Capability)
}

func ParseIntegrationPermissionGrant(s string) (IntegrationPermissionGrant, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return IntegrationPermissionGrant{}, fmt.Errorf("empty integration permission")
	}

	mode := PermOn
	for _, candidate := range []PermissionLevel{PermOn, PermAsk, PermOff} {
		prefix := string(candidate) + ":"
		if strings.HasPrefix(s, prefix) {
			mode = candidate
			s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
			break
		}
	}

	if s == "" {
		return IntegrationPermissionGrant{}, fmt.Errorf("empty capability")
	}

	return IntegrationPermissionGrant{
		Mode:       mode,
		Capability: s,
	}, nil
}

func (g *IntegrationPermissionGrant) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		grant, err := ParseIntegrationPermissionGrant(node.Value)
		if err != nil {
			return err
		}
		*g = grant
		return nil
	case yaml.MappingNode:
		if len(node.Content) != 2 {
			return fmt.Errorf("integration permission mapping must have exactly one entry")
		}
		mode := PermissionLevel(strings.TrimSpace(node.Content[0].Value))
		if !validPermissionLevel(mode) {
			return fmt.Errorf("invalid integration permission level: %s", mode)
		}
		capability := strings.TrimSpace(node.Content[1].Value)
		if capability == "" {
			return fmt.Errorf("integration permission capability cannot be empty")
		}
		*g = IntegrationPermissionGrant{
			Mode:       mode,
			Capability: capability,
		}
		return nil
	default:
		return fmt.Errorf("integration permission must be a string or single-entry mapping")
	}
}

func (g *IntegrationPermissionGrant) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("empty integration permission json")
	}

	if data[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		grant, err := ParseIntegrationPermissionGrant(raw)
		if err != nil {
			return err
		}
		*g = grant
		return nil
	}

	type alias IntegrationPermissionGrant
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Mode == "" {
		decoded.Mode = PermOn
	}
	if !validPermissionLevel(decoded.Mode) {
		return fmt.Errorf("invalid integration permission level: %s", decoded.Mode)
	}
	if strings.TrimSpace(decoded.Capability) == "" {
		return fmt.Errorf("integration permission capability cannot be empty")
	}
	*g = IntegrationPermissionGrant(decoded)
	return nil
}

func (p *IntegrationPermissions) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode, yaml.MappingNode:
		var grant IntegrationPermissionGrant
		if err := grant.UnmarshalYAML(node); err != nil {
			return err
		}
		*p = IntegrationPermissions{grant}
		return nil
	case yaml.SequenceNode:
		result := make(IntegrationPermissions, 0, len(node.Content))
		for _, item := range node.Content {
			var grant IntegrationPermissionGrant
			if err := grant.UnmarshalYAML(item); err != nil {
				return err
			}
			result = append(result, grant)
		}
		*p = result
		return nil
	default:
		return fmt.Errorf("integration permissions must be a string, mapping, or list")
	}
}

func (p *IntegrationPermissions) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("empty integration permissions json")
	}

	switch data[0] {
	case '"', '{':
		var grant IntegrationPermissionGrant
		if err := grant.UnmarshalJSON(data); err != nil {
			return err
		}
		*p = IntegrationPermissions{grant}
		return nil
	case '[':
		var raw []json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		result := make(IntegrationPermissions, 0, len(raw))
		for _, item := range raw {
			var grant IntegrationPermissionGrant
			if err := grant.UnmarshalJSON(item); err != nil {
				return err
			}
			result = append(result, grant)
		}
		*p = result
		return nil
	default:
		return fmt.Errorf("invalid integration permissions json")
	}
}
