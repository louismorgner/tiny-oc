package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tiny-oc/toc/internal/integration"
)

func TestSmoke_IntegrationGateway(t *testing.T) {
	// Start a fake API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header was sent.
		if r.Header.Get("Authorization") != "Bearer test-token-123" {
			t.Errorf("expected Bearer auth header, got: %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]string{
				{"id": "1", "name": "item-one", "secret": "should-be-filtered"},
			},
			"total":  1,
			"hidden": "should-be-filtered",
		})
	}))
	defer server.Close()

	// Build an integration definition pointing to our test server.
	def := &integration.Definition{
		Name:        "testapi",
		Description: "Test API for e2e",
		Auth:        integration.AuthConfig{Method: "token"},
		Actions: map[string]integration.Action{
			"items.list": {
				Description: "List items",
				Method:      "GET",
				Endpoint:    server.URL + "/api/items",
				AuthHeader:  "Bearer {{token}}",
				Returns:     []string{"items", "total"},
			},
		},
	}

	cred := &integration.Credential{
		AccessToken: "test-token-123",
	}

	// Invoke through the gateway.
	resp, err := integration.Invoke(&integration.InvokeRequest{
		SessionID:   "test-session-001",
		Integration: "testapi",
		Action:      "items.list",
		Params:      map[string]string{},
		Credential:  cred,
		Definition:  def,
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	// Verify status code.
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got: %d", resp.StatusCode)
	}

	// Verify response was filtered (items and total present, no hidden/secret).
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got: %T", resp.Data)
	}
	if _, ok := data["items"]; !ok {
		t.Error("expected 'items' in filtered response")
	}
	if _, ok := data["total"]; !ok {
		t.Error("expected 'total' in filtered response")
	}
	if _, ok := data["hidden"]; ok {
		t.Error("'hidden' field should have been filtered out")
	}

	// Verify array items are present (items array is returned as-is since
	// the whitelist specifies "items" at the top level, not individual fields).
	items, _ := data["items"].([]interface{})
	if len(items) != 1 {
		t.Errorf("expected 1 item, got: %d", len(items))
	}

	// Test permission checking directly.
	perms, err := integration.ParsePermissions([]string{"items.*:*"})
	if err != nil {
		t.Fatalf("ParsePermissions failed: %v", err)
	}

	// items.list should be allowed.
	if !integration.CheckPermission(perms, "items.list", "*") {
		t.Error("items.list should be allowed with items.*:* permission")
	}

	// admin.delete should be denied.
	if integration.CheckPermission(perms, "admin.delete", "*") {
		t.Error("admin.delete should be denied with items.*:* permission")
	}
}
