package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Definition represents an integration definition loaded from YAML.
type Definition struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Auth        AuthConfig        `yaml:"auth"`
	Actions     map[string]Action `yaml:"actions"`
}

// AuthConfig describes how authentication works for this integration.
type AuthConfig struct {
	Method         string   `yaml:"method"` // oauth2, api_key, token
	SetupURL       string   `yaml:"setup_url,omitempty"`
	RequiredScopes []string `yaml:"required_scopes,omitempty"`
}

// Action describes a single API action an agent can invoke.
type Action struct {
	Description string            `yaml:"description"`
	Scopes      map[string]string `yaml:"scopes,omitempty"`
	Params      []Param           `yaml:"params,omitempty"`
	Method      string            `yaml:"method"` // GET, POST, PUT, DELETE, PATCH
	Endpoint    string            `yaml:"endpoint"`
	AuthHeader  string            `yaml:"auth_header"`
	BodyFormat  string            `yaml:"body_format"` // json, query, form
	RateLimit   *RateLimit        `yaml:"rate_limit,omitempty"`
	Returns     []string          `yaml:"returns,omitempty"`
}

// Param describes a parameter for an action.
type Param struct {
	Name     string `yaml:"name"`
	Required bool   `yaml:"required"`
	Default  string `yaml:"default,omitempty"`
}

// RateLimit defines per-action rate limiting.
type RateLimit struct {
	Max    int           `yaml:"max"`
	Window time.Duration `yaml:"window"`
}

// LoadDefinition reads and validates an integration definition from a YAML file.
func LoadDefinition(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read integration definition: %w", err)
	}
	var def Definition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse integration definition: %w", err)
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	return &def, nil
}

// LoadFromRegistry loads an integration definition from the built-in registry.
// It searches relative to the binary or from a known registry path.
func LoadFromRegistry(name string) (*Definition, error) {
	// Try registry/integrations/<name>/integration.yaml relative to executable
	exePath, err := os.Executable()
	if err == nil {
		registryPath := filepath.Join(filepath.Dir(exePath), "..", "registry", "integrations", name, "integration.yaml")
		if def, err := LoadDefinition(registryPath); err == nil {
			return def, nil
		}
	}

	// Try relative to working directory (for development)
	candidates := []string{
		filepath.Join("registry", "integrations", name, "integration.yaml"),
	}
	for _, path := range candidates {
		if def, err := LoadDefinition(path); err == nil {
			return def, nil
		}
	}

	return nil, fmt.Errorf("integration '%s' not found in registry", name)
}

// Validate checks the definition for errors.
func (d *Definition) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("integration definition missing name")
	}
	if d.Auth.Method == "" {
		return fmt.Errorf("integration '%s': missing auth.method", d.Name)
	}
	validMethods := map[string]bool{"oauth2": true, "api_key": true, "token": true}
	if !validMethods[d.Auth.Method] {
		return fmt.Errorf("integration '%s': unknown auth method '%s' (expected oauth2, api_key, or token)", d.Name, d.Auth.Method)
	}
	if len(d.Actions) == 0 {
		return fmt.Errorf("integration '%s': no actions defined", d.Name)
	}
	for name, action := range d.Actions {
		if err := validateAction(d.Name, name, action); err != nil {
			return err
		}
	}
	return nil
}

func validateAction(integration, name string, a Action) error {
	if a.Method == "" {
		return fmt.Errorf("integration '%s' action '%s': missing method", integration, name)
	}
	validHTTP := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}
	if !validHTTP[a.Method] {
		return fmt.Errorf("integration '%s' action '%s': invalid HTTP method '%s'", integration, name, a.Method)
	}
	if a.Endpoint == "" {
		return fmt.Errorf("integration '%s' action '%s': missing endpoint", integration, name)
	}
	if a.AuthHeader == "" {
		return fmt.Errorf("integration '%s' action '%s': missing auth_header", integration, name)
	}
	return nil
}

// HasAction checks if the integration defines a given action.
func (d *Definition) HasAction(action string) bool {
	_, ok := d.Actions[action]
	return ok
}

// GetAction returns the action definition or an error if not found.
func (d *Definition) GetAction(action string) (*Action, error) {
	a, ok := d.Actions[action]
	if !ok {
		return nil, fmt.Errorf("integration '%s' has no action '%s'", d.Name, action)
	}
	return &a, nil
}
