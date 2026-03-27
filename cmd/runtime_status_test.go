package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	"gopkg.in/yaml.v3"
)

func TestStatusJSONForSessionIncludesTodos(t *testing.T) {
	sess := &session.Session{
		ID:          "sess-status-json",
		Agent:       "native-agent",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}
	if err := runtime.SaveState(sess, &runtime.State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Agent:     sess.Agent,
		Model:     "openai/gpt-4o-mini",
		Todos: []runtime.TodoItem{
			{Content: "Implement TodoWrite", Status: "in_progress", Priority: "high"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	got := statusJSONForSession(sess)
	if len(got.Todos) != 1 {
		t.Fatalf("len(got.Todos) = %d, want 1", len(got.Todos))
	}
	if got.Todos[0].Content != "Implement TodoWrite" {
		t.Fatalf("unexpected todos: %#v", got.Todos)
	}
}

func TestShowSubAgentStatusDisplaysTodos(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".toc"), 0755); err != nil {
		t.Fatal(err)
	}
	metadataDir := filepath.Join(workspace, ".toc", "sessions", "child-status")
	sessions := session.SessionsFile{
		Sessions: []session.Session{
			{
				ID:              "child-status",
				Agent:           "native-agent",
				Runtime:         runtimeinfo.NativeRuntime,
				MetadataDir:     metadataDir,
				CreatedAt:       time.Now(),
				WorkspacePath:   t.TempDir(),
				Status:          session.StatusCompletedOK,
				ParentSessionID: "parent-status",
			},
		},
	}
	data, err := yaml.Marshal(&sessions)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".toc", "sessions.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(&session.Session{
		ID:          "child-status",
		MetadataDir: metadataDir,
	}, &runtime.State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: "child-status",
		Agent:     "native-agent",
		Model:     "openai/gpt-4o-mini",
		Todos: []runtime.TodoItem{
			{Content: "Check final output", Status: "pending", Priority: "medium"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := showSubAgentStatus(&runtime.Context{Workspace: workspace, SessionID: "parent-status"}, "child-status"); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Todos:") {
		t.Fatalf("expected todos section in output: %q", output)
	}
	if !strings.Contains(output, "Check final output") {
		t.Fatalf("expected todo content in output: %q", output)
	}
}
