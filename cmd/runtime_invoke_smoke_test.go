package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/runtime"
)

func TestRuntimeInvokeSlackSmoke_CapabilityAllowsPost(t *testing.T) {
	var gotChannel string
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		defer r.Body.Close()
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotChannel, _ = body["channel"].(string)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"channel": gotChannel,
			"ts":      "123.456",
			"message": map[string]interface{}{
				"text": "hello",
				"user": "U123",
			},
		})
	}))
	defer server.Close()

	workspace := t.TempDir()
	writeSlackTestRegistry(t, workspace, server.URL)
	writePermissionManifestForTest(t, workspace, "sess-1", "test-agent", agent.IntegrationPermissions{
		{Mode: agent.PermOn, Capability: "post:channels/*"},
	})
	if err := integration.StoreCredentialInWorkspace(workspace, "slack", &integration.Credential{
		AccessToken: "xoxp-test",
	}); err != nil {
		t.Fatal(err)
	}

	withRuntimeEnv(t, workspace, "test-agent", "sess-1", func() {
		withWorkingDir(t, workspace, func() {
			out := captureStdout(t, func() {
				if err := runtimeInvokeCmd.RunE(runtimeInvokeCmd, []string{"slack", "send_message", "--channel", "C123", "--text", "hello"}); err != nil {
					t.Fatal(err)
				}
			})
			if !strings.Contains(out, `"status_code": 200`) {
				t.Fatalf("expected success output, got %s", out)
			}
		})
	})

	if gotChannel != "C123" {
		t.Fatalf("expected canonical channel id to be sent, got %q", gotChannel)
	}
	if gotAuth != "Bearer xoxp-test" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
}

func TestRuntimeInvokeSlackSmoke_AskAllowAlwaysWritesOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"channel": "C123",
			"ts":      "123.456",
			"message": map[string]interface{}{"text": "hello", "user": "U123"},
		})
	}))
	defer server.Close()

	workspace := t.TempDir()
	writeSlackTestRegistry(t, workspace, server.URL)
	writePermissionManifestForTest(t, workspace, "sess-ask", "test-agent", agent.IntegrationPermissions{
		{Mode: agent.PermAsk, Capability: "post:channels/*"},
	})
	if err := integration.StoreCredentialInWorkspace(workspace, "slack", &integration.Credential{
		AccessToken: "xoxp-test",
	}); err != nil {
		t.Fatal(err)
	}

	go writeApprovalResponseWhenReady(t, workspace, "sess-ask", "allow_always")

	withRuntimeEnv(t, workspace, "test-agent", "sess-ask", func() {
		withWorkingDir(t, workspace, func() {
			if err := runtimeInvokeCmd.RunE(runtimeInvokeCmd, []string{"slack", "send_message", "--channel", "C123", "--text", "hello"}); err != nil {
				t.Fatal(err)
			}
		})
	})

	manifest, err := runtime.LoadPermissionManifestInWorkspace(workspace, "sess-ask")
	if err != nil {
		t.Fatal(err)
	}
	grants := manifest.Integrations["slack"]
	found := false
	for _, grant := range grants {
		if grant.Mode == agent.PermOn && grant.Capability == "post:id/C123" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session-scoped override post:id/C123 in manifest, got %#v", grants)
	}
}

func TestRuntimeInvokeSlackSmoke_OffPermissionBlocksBeforeHTTP(t *testing.T) {
	serverHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHit = true
		http.NotFound(w, r)
	}))
	defer server.Close()

	workspace := t.TempDir()
	writeSlackTestRegistry(t, workspace, server.URL)
	writePermissionManifestForTest(t, workspace, "sess-off", "test-agent", agent.IntegrationPermissions{
		{Mode: agent.PermOff, Capability: "post:channels/*"},
	})
	if err := integration.StoreCredentialInWorkspace(workspace, "slack", &integration.Credential{
		AccessToken: "xoxp-test",
	}); err != nil {
		t.Fatal(err)
	}

	withRuntimeEnv(t, workspace, "test-agent", "sess-off", func() {
		withWorkingDir(t, workspace, func() {
			err := runtimeInvokeCmd.RunE(runtimeInvokeCmd, []string{"slack", "send_message", "--channel", "C123", "--text", "hello"})
			if err == nil || !strings.Contains(err.Error(), "permission denied") {
				t.Fatalf("expected permission denied error, got %v", err)
			}
		})
	})

	if serverHit {
		t.Fatal("expected permission denial before any HTTP request")
	}
}

func TestAppendSessionPermissionOverride_Concurrent(t *testing.T) {
	workspace := t.TempDir()
	writePermissionManifestForTest(t, workspace, "sess-lock", "test-agent", agent.IntegrationPermissions{
		{Mode: agent.PermAsk, Capability: "post:channels/*"},
	})
	ctx := &runtime.Context{
		Workspace: workspace,
		Agent:     "test-agent",
		SessionID: "sess-lock",
	}

	var wg sync.WaitGroup
	for _, id := range []string{"C111", "C222"} {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := appendSessionPermissionOverride(ctx, "slack", "post", integration.PermissionTarget{
				ID:    id,
				Exact: "id/" + id,
			}); err != nil {
				t.Errorf("appendSessionPermissionOverride(%s): %v", id, err)
			}
		}()
	}
	wg.Wait()

	manifest, err := runtime.LoadPermissionManifestInWorkspace(workspace, "sess-lock")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, grant := range manifest.Integrations["slack"] {
		got[grant.Capability] = true
	}
	for _, capability := range []string{"post:id/C111", "post:id/C222"} {
		if !got[capability] {
			t.Fatalf("expected concurrent override %s to persist, got %#v", capability, manifest.Integrations["slack"])
		}
	}
}

func writeSlackTestRegistry(t *testing.T, workspace, baseURL string) {
	t.Helper()

	content := `name: slack
description: Slack API test registry
auth:
  method: oauth2
  user_scopes:
    - channels:read
    - groups:read
    - im:read
    - mpim:read
    - channels:history
    - groups:history
    - im:history
    - mpim:history
    - chat:write
    - reactions:write
    - search:read
capabilities:
  post:
    description: Send messages
    actions:
      - send_message
  discover:
    description: List conversations
    actions:
      - list_channels
actions:
  send_message:
    description: Send a message
    scopes:
      "*": any visible conversation
      "channels/*": any public or private channel
      "id/C123": one exact Slack conversation ID
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
  list_channels:
    description: List conversations
    scopes:
      "*": all visible conversations
    params:
      - name: limit
        required: false
        default: "100"
      - name: types
        required: false
        default: public_channel,private_channel,im,mpim
    method: GET
    endpoint: ` + baseURL + `/conversations.list
    auth_header: "Bearer {{token}}"
    body_format: query
    returns:
      - ok
      - channels[].id
      - channels[].name
`

	path := filepath.Join(workspace, "registry", "integrations", "slack", "integration.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func writePermissionManifestForTest(t *testing.T, workspace, sessionID, agentName string, grants agent.IntegrationPermissions) {
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

func withRuntimeEnv(t *testing.T, workspace, agentName, sessionID string, fn func()) {
	t.Helper()
	oldWorkspace, hadWorkspace := os.LookupEnv("TOC_WORKSPACE")
	oldAgent, hadAgent := os.LookupEnv("TOC_AGENT")
	oldSessionID, hadSessionID := os.LookupEnv("TOC_SESSION_ID")

	if err := os.Setenv("TOC_WORKSPACE", workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("TOC_AGENT", agentName); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("TOC_SESSION_ID", sessionID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		restoreEnv("TOC_WORKSPACE", oldWorkspace, hadWorkspace)
		restoreEnv("TOC_AGENT", oldAgent, hadAgent)
		restoreEnv("TOC_SESSION_ID", oldSessionID, hadSessionID)
	}()

	fn()
}

func restoreEnv(key, value string, had bool) {
	if had {
		_ = os.Setenv(key, value)
		return
	}
	_ = os.Unsetenv(key)
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(old)
	}()
	fn()
}

func writeApprovalResponseWhenReady(t *testing.T, workspace, sessionID, decision string) {
	t.Helper()

	dir := filepath.Join(workspace, ".toc", "sessions", sessionID, "pending_approvals")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".json") && !strings.HasSuffix(entry.Name(), ".response.json") {
					responsePath := filepath.Join(dir, strings.TrimSuffix(entry.Name(), ".json")+".response.json")
					resp := runtime.PendingApprovalResponse{Decision: decision}
					data, err := json.Marshal(resp)
					if err != nil {
						t.Error(err)
						return
					}
					if err := os.WriteFile(responsePath, data, 0600); err != nil {
						t.Error(err)
					}
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Error("timed out waiting for pending approval request")
}
