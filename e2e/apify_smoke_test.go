package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tiny-oc/toc/internal/integration"
)

// TestSmoke_ApifyRunActor validates the full gateway flow for the Apify
// run_actor action — endpoint templating, auth, JSON body construction,
// and response pass-through (no whitelist).
func TestSmoke_ApifyRunActor(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		// Simulate Apify returning dataset items as a top-level JSON array.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":       "tweet-1",
				"text":     "Hello world",
				"username": "testuser",
				"likes":    42,
			},
			{
				"id":       "tweet-2",
				"text":     "Another tweet",
				"username": "testuser",
				"likes":    7,
			},
		})
	}))
	defer server.Close()

	def := &integration.Definition{
		Name:        "apify",
		Description: "Apify API for e2e test",
		Auth:        integration.AuthConfig{Method: "api_key"},
		Actions: map[string]integration.Action{
			"run_actor": {
				Description: "Run any Apify actor synchronously",
				Params: []integration.Param{
					{Name: "actorId", Required: true},
					{Name: "input", Required: true, Kind: "json", BodyPath: "."},
				},
				Method:     "POST",
				Endpoint:   server.URL + "/v2/acts/{{actorId}}/run-sync-get-dataset-items",
				AuthHeader: "Bearer {{token}}",
				BodyFormat: "json",
				// No Returns whitelist — Apify actors have varied schemas.
			},
		},
	}

	resp, err := integration.Invoke(&integration.InvokeRequest{
		SessionID:   "test-session-apify",
		Integration: "apify",
		Action:      "run_actor",
		Params: map[string]string{
			"actorId": "apidojo~tweet-scraper",
			"input":   `{"startUrls":[{"url":"https://x.com/elonmusk"}],"maxTweets":5}`,
		},
		Credential: &integration.Credential{AccessToken: "apft_test-key-123"},
		Definition: def,
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	// 1. Verify actorId was templated into the URL path.
	//    Apify uses "~" as the namespace separator in API URLs.
	expectedPath := "/v2/acts/apidojo~tweet-scraper/run-sync-get-dataset-items"
	if gotPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, gotPath)
	}

	// 2. Verify auth header.
	if gotAuth != "Bearer apft_test-key-123" {
		t.Errorf("expected Bearer auth, got %q", gotAuth)
	}

	// 3. Verify the JSON body contains the actor input at the TOP LEVEL,
	//    not wrapped in {"input": {...}}.
	//
	//    Apify's run-sync-get-dataset-items endpoint expects the actor input
	//    as the request body directly. If the gateway wraps it in an "input"
	//    envelope, the actor receives empty/default input and returns nothing.
	if _, wrapped := gotBody["input"]; wrapped {
		t.Fatalf("BUG: request body wraps actor input in {\"input\": ...} envelope.\n"+
			"Apify expects the input as the top-level body.\n"+
			"Got body: %v", gotBody)
	}

	startUrls, ok := gotBody["startUrls"]
	if !ok {
		t.Fatalf("expected startUrls at top level of body, got: %v", gotBody)
	}
	urls, ok := startUrls.([]interface{})
	if !ok || len(urls) != 1 {
		t.Fatalf("expected startUrls to be array with 1 element, got: %v", startUrls)
	}

	if gotBody["maxTweets"] != float64(5) {
		t.Errorf("expected maxTweets=5, got: %v", gotBody["maxTweets"])
	}

	// 4. Verify response — no whitelist, so full array should be returned.
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	items, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected array response (Apify returns dataset items as JSON array), got: %T", resp.Data)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}

	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first item to be map, got: %T", items[0])
	}
	if first["text"] != "Hello world" {
		t.Errorf("expected first tweet text, got: %v", first["text"])
	}
}

// TestSmoke_ApifyRunActor_LiveAPI hits the real Apify API with the tweet-scraper
// actor. Only runs when APIFY_TEST_TOKEN is set. This catches issues that mock
// tests miss: auth format, endpoint URL correctness, timeout behavior, and
// response schema assumptions.
func TestSmoke_ApifyRunActor_LiveAPI(t *testing.T) {
	token := os.Getenv("APIFY_TEST_TOKEN")
	if token == "" {
		t.Skip("APIFY_TEST_TOKEN not set — skipping live API test")
	}

	def := &integration.Definition{
		Name:        "apify",
		Description: "Apify API",
		Auth:        integration.AuthConfig{Method: "api_key"},
		Actions: map[string]integration.Action{
			"run_actor": {
				Description: "Run any Apify actor synchronously",
				Params: []integration.Param{
					{Name: "actorId", Required: true},
					{Name: "input", Required: true, Kind: "json", BodyPath: "."},
				},
				Method:     "POST",
				Endpoint:   "https://api.apify.com/v2/acts/{{actorId}}/run-sync-get-dataset-items",
				AuthHeader: "Bearer {{token}}",
				BodyFormat: "json",
			},
		},
	}

	resp, err := integration.Invoke(&integration.InvokeRequest{
		SessionID:   "live-test",
		Integration: "apify",
		Action:      "run_actor",
		Params: map[string]string{
			"actorId": "apidojo~tweet-scraper",
			"input":   `{"startUrls":[{"url":"https://x.com/elonmusk"}],"maxItems":2}`,
		},
		Credential: &integration.Credential{AccessToken: token},
		Definition: def,
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	// Apify returns 201 for newly created actor runs.
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("expected 200 or 201, got %d — error: %s", resp.StatusCode, resp.Error)
	}

	// Apify returns dataset items as a top-level JSON array.
	items, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected JSON array response, got %T: %v", resp.Data, resp.Data)
	}
	if len(items) == 0 {
		t.Fatal("expected at least 1 tweet item from live API")
	}

	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected item to be map, got %T", items[0])
	}

	// Sanity check: tweet items should have text content.
	hasContent := false
	for _, key := range []string{"text", "full_text", "tweet_text"} {
		if v, ok := first[key]; ok && v != nil {
			hasContent = true
			break
		}
	}
	if !hasContent {
		t.Logf("WARNING: tweet item has no recognizable text field. Keys: %v", mapKeys(first))
	}

	t.Logf("Live API returned %d items. First item keys: %v", len(items), mapKeys(first))
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
