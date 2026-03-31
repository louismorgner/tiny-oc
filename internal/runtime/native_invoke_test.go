package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/integration"
)

// useTestMasterKey overrides integration.MasterKeyFunc with a static in-memory
// key for the duration of the test, avoiding OS keychain access.
func useTestMasterKey(t *testing.T) {
	t.Helper()
	original := integration.MasterKeyFunc
	testKey := []byte("test-master-key-0123456789abcdef")
	integration.MasterKeyFunc = func() ([]byte, error) { return testKey, nil }
	t.Cleanup(func() { integration.MasterKeyFunc = original })
}

// setupInvokeTest creates a workspace with a mock Slack registry, permission
// manifest, and stored credential. It returns the nativeToolContext and a
// cleanup-registered workspace path.
func setupInvokeTest(t *testing.T, serverURL string, sessionID string, grants agent.IntegrationPermissions) nativeToolContext {
	t.Helper()
	useTestMasterKey(t)

	workspace := t.TempDir()
	writeTestSlackRegistry(t, workspace, serverURL)
	writeTestPermissionManifest(t, workspace, sessionID, "test-agent", grants)

	if err := integration.StoreCredentialInWorkspace(workspace, "slack", &integration.Credential{
		AccessToken: "xoxp-test",
	}); err != nil {
		t.Fatal(err)
	}

	manifest, err := LoadPermissionManifestInWorkspace(workspace, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Change working directory so LoadFromRegistry finds the local registry.
	origDir, _ := os.Getwd()
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	return nativeToolContext{
		SessionDir: filepath.Join(workspace, ".toc", "sessions", sessionID),
		Workspace:  workspace,
		Agent:      "test-agent",
		SessionID:  sessionID,
		Manifest:   manifest,
	}
}

func writeTestSlackRegistry(t *testing.T, workspace, baseURL string) {
	t.Helper()
	content := `name: slack
description: Slack API test registry
auth:
  method: oauth2
  user_scopes:
    - chat:write
capabilities:
  post:
    description: Send messages
    actions:
      - send_message
actions:
  send_message:
    description: Send a message
    scopes:
      "*": any visible conversation
      "channels/*": any public or private channel
    params:
      - name: channel
        required: true
      - name: text
        required: true
    method: POST
    endpoint: ` + baseURL + `/chat.postMessage
    auth_header: "Bearer {{token}}"
    body_format: json
    returns:
      - ok
      - channel
      - ts
      - message.text
      - message.user
`
	path := filepath.Join(workspace, "registry", "integrations", "slack", "integration.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeTestPermissionManifest(t *testing.T, workspace, sessionID, agentName string, grants agent.IntegrationPermissions) {
	t.Helper()
	dir := filepath.Join(workspace, ".toc", "sessions", sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := integration.PermissionManifest{
		SessionID: sessionID,
		Agent:     agentName,
		Integrations: map[string]agent.IntegrationPermissions{
			"slack": grants,
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "permissions.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestNativeInvoke_Success(t *testing.T) {
	var gotChannel, gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		defer r.Body.Close()
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotChannel, _ = body["channel"].(string)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"channel": gotChannel,
			"ts":      "123.456",
			"message": map[string]interface{}{"text": "hello", "user": "U123"},
		})
	}))
	defer server.Close()

	ctx := setupInvokeTest(t, server.URL, "sess-invoke-ok", agent.IntegrationPermissions{
		{Mode: agent.PermOn, Capability: "post:channels/*"},
	})

	args, _ := json.Marshal(map[string]interface{}{
		"integration": "slack",
		"action":      "send_message",
		"params":      map[string]string{"channel": "C123", "text": "hello"},
	})
	call := ToolCall{
		ID:       "call-invoke-1",
		Function: ToolCallFunction{Name: "Invoke", Arguments: string(args)},
	}

	result := nativeInvoke(ctx, call)

	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("expected success, got failure: %s", result.Message)
	}
	if !strings.Contains(result.Message, `"status_code": 200`) {
		t.Fatalf("expected status_code 200 in output, got %s", result.Message)
	}
	if gotChannel != "C123" {
		t.Fatalf("expected channel C123 sent to server, got %q", gotChannel)
	}
	if gotAuth != "Bearer xoxp-test" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
}

func TestNativeInvoke_PermissionDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("HTTP request should not be made when permission is denied")
	}))
	defer server.Close()

	ctx := setupInvokeTest(t, server.URL, "sess-invoke-off", agent.IntegrationPermissions{
		{Mode: agent.PermOff, Capability: "post:channels/*"},
	})

	args, _ := json.Marshal(map[string]interface{}{
		"integration": "slack",
		"action":      "send_message",
		"params":      map[string]string{"channel": "C123", "text": "hello"},
	})
	call := ToolCall{
		ID:       "call-invoke-denied",
		Function: ToolCallFunction{Name: "Invoke", Arguments: string(args)},
	}

	result := nativeInvoke(ctx, call)

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure, got success: %s", result.Message)
	}
	if !strings.Contains(result.Message, "does not have permission") {
		t.Fatalf("expected permission denied message, got %q", result.Message)
	}
}

func TestNativeInvoke_NoIntegrationInManifest(t *testing.T) {
	workspace := t.TempDir()
	sessionID := "sess-invoke-noint"

	// Create a manifest with no integrations.
	dir := filepath.Join(workspace, ".toc", "sessions", sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := &integration.PermissionManifest{
		SessionID:    sessionID,
		Agent:        "test-agent",
		Integrations: map[string]agent.IntegrationPermissions{},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "permissions.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	ctx := nativeToolContext{
		SessionDir: dir,
		Workspace:  workspace,
		Agent:      "test-agent",
		SessionID:  sessionID,
		Manifest:   manifest,
	}

	args, _ := json.Marshal(map[string]interface{}{
		"integration": "slack",
		"action":      "send_message",
		"params":      map[string]string{"channel": "C123", "text": "hello"},
	})
	call := ToolCall{
		ID:       "call-invoke-noint",
		Function: ToolCallFunction{Name: "Invoke", Arguments: string(args)},
	}

	result := nativeInvoke(ctx, call)

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure, got success: %s", result.Message)
	}
	if !strings.Contains(result.Message, "no permissions") {
		t.Fatalf("expected no-permission message, got %q", result.Message)
	}
}

func TestNativeInvoke_MissingRequiredParams(t *testing.T) {
	args, _ := json.Marshal(map[string]interface{}{
		"integration": "",
		"action":      "send_message",
		"params":      map[string]string{},
	})
	call := ToolCall{
		ID:       "call-invoke-noparam",
		Function: ToolCallFunction{Name: "Invoke", Arguments: string(args)},
	}

	ctx := nativeToolContext{Agent: "test-agent"}
	result := nativeInvoke(ctx, call)

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure for empty integration, got success")
	}
	if !strings.Contains(result.Message, "integration is required") {
		t.Fatalf("expected 'integration is required' message, got %q", result.Message)
	}
}

func TestNativeInvoke_NilManifest(t *testing.T) {
	args, _ := json.Marshal(map[string]interface{}{
		"integration": "slack",
		"action":      "send_message",
		"params":      map[string]string{"channel": "C123", "text": "hi"},
	})
	call := ToolCall{
		ID:       "call-invoke-nilmanifest",
		Function: ToolCallFunction{Name: "Invoke", Arguments: string(args)},
	}

	ctx := nativeToolContext{Agent: "test-agent", Manifest: nil}
	result := nativeInvoke(ctx, call)

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure for nil manifest, got success")
	}
	if !strings.Contains(result.Message, "no permission manifest") {
		t.Fatalf("expected manifest error, got %q", result.Message)
	}
}

func TestNativeInvoke_DoesNotCheckFilesystemExecute(t *testing.T) {
	// This is the core bug fix: Invoke should work even when filesystem.execute is off.
	var gotChannel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotChannel, _ = body["channel"].(string)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"channel": gotChannel,
			"ts":      "789.012",
			"message": map[string]interface{}{"text": "works", "user": "U456"},
		})
	}))
	defer server.Close()

	useTestMasterKey(t)
	workspace := t.TempDir()
	sessionID := "sess-invoke-noexec"
	writeTestSlackRegistry(t, workspace, server.URL)

	// Create manifest with filesystem.execute OFF and integration ON.
	dir := filepath.Join(workspace, ".toc", "sessions", sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := &integration.PermissionManifest{
		SessionID: sessionID,
		Agent:     "test-agent",
		Filesystem: agent.FilesystemPermissions{
			Read:    agent.PermOn,
			Write:   agent.PermOn,
			Execute: agent.PermOff,
		},
		Integrations: map[string]agent.IntegrationPermissions{
			"slack": {
				{Mode: agent.PermOn, Capability: "post:channels/*"},
			},
		},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "permissions.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	if err := integration.StoreCredentialInWorkspace(workspace, "slack", &integration.Credential{
		AccessToken: "xoxp-test",
	}); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	ctx := nativeToolContext{
		SessionDir: dir,
		Workspace:  workspace,
		Agent:      "test-agent",
		SessionID:  sessionID,
		Manifest:   manifest,
	}

	// Verify Bash is blocked by filesystem.execute off.
	bashErr := ValidateFilesystemPermission(manifest, "execute", "test-agent")
	if bashErr == nil {
		t.Fatal("expected filesystem execute to be denied, but it was allowed")
	}

	// Verify Invoke works despite filesystem.execute being off.
	args, _ := json.Marshal(map[string]interface{}{
		"integration": "slack",
		"action":      "send_message",
		"params":      map[string]string{"channel": "C123", "text": "works"},
	})
	call := ToolCall{
		ID:       "call-invoke-noexec",
		Function: ToolCallFunction{Name: "Invoke", Arguments: string(args)},
	}

	result := nativeInvoke(ctx, call)

	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("expected Invoke to succeed with filesystem.execute off, got failure: %s", result.Message)
	}
	if gotChannel != "C123" {
		t.Fatalf("expected channel C123, got %q", gotChannel)
	}
}
