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

func TestSmoke_ExaIntegrationGateway(t *testing.T) {
	// Start a fake Exa API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify x-api-key header (not Authorization).
		if r.Header.Get("x-api-key") != "exa-test-key-123" {
			t.Errorf("expected x-api-key header 'exa-test-key-123', got: %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header for Exa, got: %q", r.Header.Get("Authorization"))
		}

		// Verify the request body is properly typed JSON.
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		// query should be a string
		if body["query"] != "semantic search test" {
			t.Errorf("expected query 'semantic search test', got: %v", body["query"])
		}

		// numResults should be a number (float64 from JSON)
		if body["numResults"] != float64(3) {
			t.Errorf("expected numResults 3, got: %v (%T)", body["numResults"], body["numResults"])
		}

		// contents should be a nested object with highlights: true
		contents, ok := body["contents"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected contents to be object, got: %T", body["contents"])
		}
		if contents["highlights"] != true {
			t.Errorf("expected contents.highlights true, got: %v", contents["highlights"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"requestId":  "req-123",
			"searchType": "neural",
			"results": []map[string]interface{}{
				{
					"title":         "Test Result",
					"url":           "https://example.com/test",
					"publishedDate": "2025-01-15",
					"author":        "Test Author",
					"highlights":    []string{"highlighted snippet"},
					"score":         0.95,
				},
			},
			"costDollars": map[string]interface{}{"total": 0.001},
		})
	}))
	defer server.Close()

	def := &integration.Definition{
		Name:        "exa",
		Description: "Exa API for e2e test",
		Auth:        integration.AuthConfig{Method: "api_key"},
		Actions: map[string]integration.Action{
			"search": {
				Description: "Semantic search",
				Method:      "POST",
				Endpoint:    server.URL + "/search",
				AuthHeader:  "x-api-key: {{token}}",
				BodyFormat:  "json",
				Returns: []string{
					"results[].title",
					"results[].url",
					"results[].publishedDate",
					"results[].author",
					"results[].highlights",
				},
			},
		},
	}

	cred := &integration.Credential{
		AccessToken: "exa-test-key-123",
	}

	resp, err := integration.Invoke(&integration.InvokeRequest{
		SessionID:   "test-session-exa",
		Integration: "exa",
		Action:      "search",
		Params: map[string]string{
			"query":      "semantic search test",
			"numResults": "3",
		},
		Credential: cred,
		Definition: def,
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got: %d", resp.StatusCode)
	}

	// Verify response was filtered.
	// The gateway's results[].field notation extracts per-field arrays into a nested map.
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got: %T", resp.Data)
	}

	results, ok := data["results"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected results to be map (flattened by field), got: %T", data["results"])
	}

	// Each field should contain an array of extracted values.
	titles, ok := results["title"].([]interface{})
	if !ok || len(titles) != 1 || titles[0] != "Test Result" {
		t.Errorf("expected results.title = ['Test Result'], got: %v", results["title"])
	}
	urls, ok := results["url"].([]interface{})
	if !ok || len(urls) != 1 || urls[0] != "https://example.com/test" {
		t.Errorf("expected results.url = ['https://example.com/test'], got: %v", results["url"])
	}

	// score should be filtered out (not in whitelist).
	if _, ok := results["score"]; ok {
		t.Error("score should have been filtered out")
	}

	// costDollars should be filtered out.
	if _, ok := data["costDollars"]; ok {
		t.Error("costDollars should have been filtered out")
	}
	// requestId should be filtered out.
	if _, ok := data["requestId"]; ok {
		t.Error("requestId should have been filtered out")
	}
}
