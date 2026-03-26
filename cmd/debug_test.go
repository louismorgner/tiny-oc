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

func TestBuildDebugReportIncludesCrashArtifacts(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	workDir := t.TempDir()
	metaDir := t.TempDir()
	sess := &session.Session{
		ID:            "sess-debug",
		Agent:         "native-agent",
		Runtime:       runtimeinfo.NativeRuntime,
		MetadataDir:   metaDir,
		CreatedAt:     time.Now().Add(-2 * time.Minute),
		WorkspacePath: workDir,
	}
	if err := runtime.SaveState(sess, &runtime.State{
		Runtime:       runtimeinfo.NativeRuntime,
		SessionID:     sess.ID,
		Agent:         sess.Agent,
		Model:         "openai/gpt-4o-mini",
		Status:        "crashed",
		LastError:     "panic captured",
		ResumeCount:   2,
		RecoveryCount: 1,
		CrashInfo: &runtime.CrashInfo{
			PanicMessage: "panic captured",
			LastToolCall: "Edit",
			CrashTime:    time.Now().UTC(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveEventLog(sess, &runtime.ParsedLog{
		Events: []runtime.Event{
			{Timestamp: time.Now().UTC(), Step: runtime.Step{Type: "tool", Tool: "Edit", Content: "patched file"}},
			{Timestamp: time.Now().UTC(), Step: runtime.Step{Type: "crash", Content: "panic captured", StackTrace: "goroutine 1"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "toc-output.txt"), []byte("partial output"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.StderrLogPath(sess), []byte("panic: panic captured"), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := buildDebugReport(sess, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.State.CrashInfo == nil || report.State.CrashInfo.LastToolCall != "Edit" {
		t.Fatalf("unexpected crash info: %#v", report.State.CrashInfo)
	}
	if report.Timeline.TotalEvents != 2 || len(report.Timeline.RecentEvents) != 1 {
		t.Fatalf("timeline = %#v", report.Timeline)
	}
	if report.Output == nil || !strings.Contains(report.Output.Content, "partial output") {
		t.Fatalf("output artifact = %#v", report.Output)
	}
	if report.Stderr == nil || !strings.Contains(report.Stderr.Content, "panic captured") {
		t.Fatalf("stderr artifact = %#v", report.Stderr)
	}

	out := captureStdout(t, func() {
		printDebugReport(report, false)
	})
	for _, want := range []string{"Exit reason:", "Timeline", "stderr.log", "panic captured"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output: %q", want, out)
		}
	}
}

func TestShowSubAgentStatusHintsDebugOnFailure(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".toc"), 0755); err != nil {
		t.Fatal(err)
	}
	sessions := session.SessionsFile{
		Sessions: []session.Session{
			{
				ID:              "child-1",
				Agent:           "native-agent",
				Runtime:         runtimeinfo.NativeRuntime,
				MetadataDir:     filepath.Join(workspace, ".toc", "sessions", "child-1"),
				CreatedAt:       time.Now(),
				WorkspacePath:   t.TempDir(),
				Status:          session.StatusCompletedError,
				ParentSessionID: "parent-1",
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

	out := captureStdout(t, func() {
		if err := showSubAgentStatus(&runtime.Context{Workspace: workspace, SessionID: "parent-1"}, "child-1"); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "toc debug child-1") {
		t.Fatalf("expected debug hint in output: %q", out)
	}
}
