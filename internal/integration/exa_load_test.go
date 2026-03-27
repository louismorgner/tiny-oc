package integration

import (
	"testing"
)

func TestLoadExaDefinition(t *testing.T) {
	def, err := LoadDefinition("../../registry/integrations/exa/integration.yaml")
	if err != nil {
		t.Fatalf("failed to load exa definition: %v", err)
	}
	if def.Name != "exa" {
		t.Errorf("expected name 'exa', got: %s", def.Name)
	}
	if def.Auth.Method != "api_key" {
		t.Errorf("expected auth method 'api_key', got: %s", def.Auth.Method)
	}
	if len(def.Actions) != 4 {
		t.Errorf("expected 4 actions, got: %d", len(def.Actions))
	}
	for _, name := range []string{"search", "find_similar", "get_contents", "extract"} {
		if _, ok := def.Actions[name]; !ok {
			t.Errorf("expected action '%s'", name)
		}
	}
	// Verify search action has correct auth header format
	search := def.Actions["search"]
	if search.AuthHeader != "x-api-key: {{token}}" {
		t.Errorf("expected auth_header 'x-api-key: {{token}}', got: %s", search.AuthHeader)
	}
}
