package inspect

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyRequestCapsCapturedResponseBody(t *testing.T) {
	capturePath := filepath.Join(t.TempDir(), "http.jsonl")
	cw, err := newCaptureWriter(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cw.Close()
	})

	body := strings.Repeat("x", maxCaptureBodyBytes+1024)
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "http://proxy.test/v1/messages", strings.NewReader(`{"model":"test"}`))
	rec := httptest.NewRecorder()
	upstream, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	proxyRequest(context.Background(), transport, cw, upstream, rec, req)
	cw.Close()

	if got := rec.Body.Len(); got != len(body) {
		t.Fatalf("response body length = %d, want %d", got, len(body))
	}

	data, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	var entry CaptureEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Response == nil {
		t.Fatal("expected captured response")
	}
	if got := len(entry.Response.Body); got != maxCaptureBodyBytes {
		t.Fatalf("captured response length = %d, want %d", got, maxCaptureBodyBytes)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
