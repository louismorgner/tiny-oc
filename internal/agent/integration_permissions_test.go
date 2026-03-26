package agent

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestIntegrationPermissions_UnmarshalScalarAndSequence(t *testing.T) {
	var perms struct {
		Permissions Permissions `yaml:"permissions"`
	}

	input := `
permissions:
  integrations:
    slack:
      - post:#eng
      - ask: post:channels/*
`

	if err := yaml.Unmarshal([]byte(input), &perms); err != nil {
		t.Fatal(err)
	}

	grants := perms.Permissions.Integrations["slack"]
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants, got %d", len(grants))
	}
	if grants[0].Mode != PermOn || grants[0].Capability != "post:#eng" {
		t.Fatalf("unexpected first grant: %#v", grants[0])
	}
	if grants[1].Mode != PermAsk || grants[1].Capability != "post:channels/*" {
		t.Fatalf("unexpected second grant: %#v", grants[1])
	}
}
