package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	inspectpkg "github.com/tiny-oc/toc/internal/inspect"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
)

func TestMostRecentInspectedSession(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	workspace := t.TempDir()
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir("/")
	})

	if err := os.MkdirAll(".toc/sessions", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".toc/config.yaml", []byte("name: test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".toc/sessions.yaml", []byte(`sessions:
- id: old-session
  agent: worker
  runtime: claude-code
  created_at: 2026-03-27T00:00:00Z
  workspace_path: /tmp/old
  metadata_dir: `+filepath.Join(workspace, ".toc", "sessions", "old-session")+`
- id: new-session
  agent: worker
  runtime: toc-native
  created_at: 2026-03-27T00:01:00Z
  workspace_path: /tmp/new
  metadata_dir: `+filepath.Join(workspace, ".toc", "sessions", "new-session")+`
`), 0600); err != nil {
		t.Fatal(err)
	}

	oldSess := &session.Session{ID: "old-session", MetadataDir: filepath.Join(workspace, ".toc", "sessions", "old-session")}
	newSess := &session.Session{ID: "new-session", MetadataDir: filepath.Join(workspace, ".toc", "sessions", "new-session")}
	if err := os.MkdirAll(filepath.Dir(runtime.InspectCapturePath(oldSess)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(runtime.InspectCapturePath(newSess)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.InspectCapturePath(oldSess), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.InspectCapturePath(newSess), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(runtime.InspectCapturePath(oldSess), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(runtime.InspectCapturePath(newSess), newTime, newTime); err != nil {
		t.Fatal(err)
	}

	got, err := mostRecentInspectedSession()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "new-session" {
		t.Fatalf("mostRecentInspectedSession() = %q, want new-session", got.ID)
	}
}

func TestPrintInspectJSONOmitsBodiesByDefault(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	sess := &session.Session{
		ID:      "sess-inspect",
		Agent:   "worker",
		Runtime: "toc-native",
	}
	report := &inspectpkg.Report{
		CapturePath: "/tmp/http.jsonl",
		Summary: inspectpkg.ReportSummary{
			CallCount:    1,
			InputTokens:  12,
			OutputTokens: 4,
			TotalTokens:  16,
			Models:       []string{"openai/gpt-4o-mini"},
		},
		Calls: []inspectpkg.CallSummary{
			{
				Index:          1,
				Method:         "POST",
				Path:           "/chat/completions",
				RequestModel:   "openai/gpt-4o-mini",
				RequestBody:    `{"model":"openai/gpt-4o-mini"}`,
				ResponseBody:   `{"model":"openai/gpt-4o-mini"}`,
				RequestHeaders: map[string][]string{"Authorization": {"[redacted]"}},
			},
		},
	}

	out := captureStdout(t, func() {
		if err := printInspectJSON(sess, report, false, false); err != nil {
			t.Fatal(err)
		}
	})

	var payload inspectJSONOutput
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Calls[0].RequestBody != "" || payload.Calls[0].ResponseBody != "" {
		t.Fatalf("expected bodies to be omitted, got %#v", payload.Calls[0])
	}
	if payload.Calls[0].RequestHeaders != nil {
		t.Fatalf("expected headers to be omitted, got %#v", payload.Calls[0].RequestHeaders)
	}
}

func TestPrintInspectHumanIncludesPromptAndBodies(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	sess := &session.Session{
		ID:      "sess-inspect",
		Agent:   "worker",
		Runtime: "claude-code",
	}
	report := &inspectpkg.Report{
		CapturePath: "/tmp/http.jsonl",
		Summary: inspectpkg.ReportSummary{
			CallCount:         1,
			TotalDurationMS:   321,
			AverageDurationMS: 321,
			InputTokens:       18,
			OutputTokens:      6,
			TotalTokens:       24,
			Models:            []string{"claude-sonnet-4"},
		},
		Calls: []inspectpkg.CallSummary{
			{
				Index:         1,
				Method:        "POST",
				Path:          "/v1/messages",
				Upstream:      "http://localhost:8000/v1/messages",
				StatusCode:    200,
				DurationMS:    321,
				RequestModel:  "claude-sonnet-4",
				PromptPreview: "fix the failing test",
				MessageCount:  3,
				ToolCount:     2,
				InputTokens:   18,
				OutputTokens:  6,
				TotalTokens:   24,
				RequestBody:   `{"model":"claude-sonnet-4"}`,
				ResponseBody:  `{"id":"msg_1"}`,
			},
		},
	}

	out := captureStdout(t, func() {
		printInspectHuman(sess, report, true, false, false)
	})

	for _, want := range []string{"Capture:", "Models:", "Calls", "#1", "prompt:", "fix the failing test", "request body:", "response body:", "321ms"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output: %q", want, out)
		}
	}
}
