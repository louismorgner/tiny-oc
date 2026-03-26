package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// helper: create a minimal workspace at root with the given config name.
func setupWorkspace(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, tocDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(&WorkspaceConfig{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, configFile), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// helper: write a secrets file at root.
func setupSecrets(t *testing.T, root string, s *Secrets) {
	t.Helper()
	dir := filepath.Join(root, tocDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, secretsFile), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceRoot(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{"defaults to dot when unset", "", "."},
		{"returns env var when set", "/some/workspace", "/some/workspace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("TOC_WORKSPACE", tt.envValue)
			} else {
				t.Setenv("TOC_WORKSPACE", "")
			}
			if got := WorkspaceRoot(); got != tt.want {
				t.Errorf("WorkspaceRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExistsIn(t *testing.T) {
	tests := []struct {
		name       string
		setup      bool // whether to create a workspace
		wantExists bool
	}{
		{"true when workspace exists", true, true},
		{"false when no workspace", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup {
				setupWorkspace(t, dir, "test")
			}
			if got := ExistsIn(dir); got != tt.wantExists {
				t.Errorf("ExistsIn(%q) = %v, want %v", dir, got, tt.wantExists)
			}
		})
	}
}

func TestLoadFrom(t *testing.T) {
	tests := []struct {
		name      string
		setup     bool
		wantName  string
		wantError bool
	}{
		{"loads config from workspace", true, "myproject", false},
		{"errors on missing workspace", false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup {
				setupWorkspace(t, dir, tt.wantName)
			}
			cfg, err := LoadFrom(dir)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", cfg.Name, tt.wantName)
			}
		})
	}
}

func TestLoadSecretsFrom(t *testing.T) {
	tests := []struct {
		name      string
		secrets   *Secrets // nil means don't create the file
		wantKey   string
		wantError bool
	}{
		{"returns empty secrets when file missing", nil, "", false},
		{"loads key from secrets file", &Secrets{OpenRouterKey: "sk-test-123"}, "sk-test-123", false},
		{"returns empty key when secrets has no key", &Secrets{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.secrets != nil {
				setupSecrets(t, dir, tt.secrets)
			}
			s, err := LoadSecretsFrom(dir)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.OpenRouterKey != tt.wantKey {
				t.Errorf("OpenRouterKey = %q, want %q", s.OpenRouterKey, tt.wantKey)
			}
		})
	}
}

func TestLoadDelegatesToLoadFrom(t *testing.T) {
	dir := t.TempDir()
	setupWorkspace(t, dir, "delegate-test")

	// Change to the temp dir so Load() (which uses ".") finds the workspace.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Name != "delegate-test" {
		t.Errorf("Load().Name = %q, want %q", cfg.Name, "delegate-test")
	}
}

func TestLoadSecretsDelegatesToLoadSecretsFrom(t *testing.T) {
	dir := t.TempDir()
	setupSecrets(t, dir, &Secrets{OpenRouterKey: "sk-delegate"})

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	s, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}
	if s.OpenRouterKey != "sk-delegate" {
		t.Errorf("OpenRouterKey = %q, want %q", s.OpenRouterKey, "sk-delegate")
	}
}

func TestLoadSecretsMissingFileBehaviorConsistent(t *testing.T) {
	// Both LoadSecrets() and LoadSecretsFrom() should return empty Secrets
	// (not an error) when the secrets file doesn't exist.
	dir := t.TempDir()

	// LoadSecretsFrom with no secrets file
	s1, err := LoadSecretsFrom(dir)
	if err != nil {
		t.Fatalf("LoadSecretsFrom() error: %v", err)
	}
	if s1.OpenRouterKey != "" {
		t.Errorf("LoadSecretsFrom() key = %q, want empty", s1.OpenRouterKey)
	}

	// LoadSecrets() in a dir with no secrets file
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	s2, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}
	if s2.OpenRouterKey != "" {
		t.Errorf("LoadSecrets() key = %q, want empty", s2.OpenRouterKey)
	}
}

func TestOpenRouterKeyDelegatesToOpenRouterKeyFrom(t *testing.T) {
	dir := t.TempDir()
	setupSecrets(t, dir, &Secrets{OpenRouterKey: "sk-or-test"})

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	if got := OpenRouterKey(); got != "sk-or-test" {
		t.Errorf("OpenRouterKey() = %q, want %q", got, "sk-or-test")
	}
}
