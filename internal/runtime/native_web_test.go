package runtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/integration"
)

func TestNativeWebFetchRequiresNetworkPermission(t *testing.T) {
	result := nativeWebFetch(nativeToolContext{
		Agent: "tester",
	}, toolCall(t, "WebFetch", map[string]interface{}{
		"url": "https://example.com",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected permission failure, got %#v", result)
	}
	if !strings.Contains(result.Message, "does not have network web access") {
		t.Fatalf("unexpected permission error: %q", result.Message)
	}
}

func TestNativeWebFetchConvertsHTMLToMarkdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head><title>Example Guide</title></head>
  <body>
    <header>site chrome</header>
    <main>
      <h1>Guide</h1>
      <p>Hello <a href="/docs/getting-started">docs</a>.</p>
      <table>
        <tr><th>Name</th><th>Value</th></tr>
        <tr><td>runtime</td><td>toc-native</td></tr>
      </table>
    </main>
  </body>
</html>`))
	}))
	defer server.Close()

	result := nativeWebFetch(nativeToolContext{
		Agent:    "tester",
		Manifest: allowWebFetchManifest(),
	}, toolCall(t, "WebFetch", map[string]interface{}{
		"url": server.URL,
	}))

	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if !strings.Contains(result.Message, "Title: Example Guide") {
		t.Fatalf("missing title metadata: %q", result.Message)
	}
	if !strings.Contains(result.Message, "# Guide") {
		t.Fatalf("expected markdown heading, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "[docs]("+server.URL+"/docs/getting-started)") {
		t.Fatalf("expected absolute markdown link, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "| Name | Value |") {
		t.Fatalf("expected markdown table, got %q", result.Message)
	}
	if strings.Contains(result.Message, "site chrome") {
		t.Fatalf("expected page chrome to be excluded, got %q", result.Message)
	}
}

func TestNativeWebFetchPrettyPrintsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"toc","count":2}`))
	}))
	defer server.Close()

	result := nativeWebFetch(nativeToolContext{
		Agent:    "tester",
		Manifest: allowWebFetchManifest(),
	}, toolCall(t, "WebFetch", map[string]interface{}{
		"url": server.URL,
	}))

	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if !strings.Contains(result.Message, "\"name\": \"toc\"") || !strings.Contains(result.Message, "\"count\": 2") {
		t.Fatalf("expected pretty-printed json, got %q", result.Message)
	}
}

func TestNativeWebFetchReportsFinalURLAfterRedirect(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("redirected"))
	})
	server.Config.Handler = mux

	result := nativeWebFetch(nativeToolContext{
		Agent:    "tester",
		Manifest: allowWebFetchManifest(),
	}, toolCall(t, "WebFetch", map[string]interface{}{
		"url": server.URL + "/start",
	}))

	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if !strings.Contains(result.Message, "Final URL: "+server.URL+"/end") {
		t.Fatalf("expected final url metadata, got %q", result.Message)
	}
}

func allowWebFetchManifest() *integration.PermissionManifest {
	return &integration.PermissionManifest{
		Agent: "tester",
		Network: agent.NetworkPermissions{
			Web: agent.PermOn,
		},
	}
}
