package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
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

func TestReadDebugArtifactLimitsContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.log")
	content := strings.Repeat("a", 140*1024)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	artifact := readDebugArtifact(path)
	if artifact == nil {
		t.Fatal("expected artifact")
	}
	if len(artifact.Content) > 128*1024 {
		t.Fatalf("artifact length = %d", len(artifact.Content))
	}
	if len(artifact.Content) == len(content) {
		t.Fatal("expected bounded content, got full file")
	}
}

func TestWriteDebugBundleIncludesExpectedEntries(t *testing.T) {
	workDir := t.TempDir()
	metaDir := t.TempDir()
	sess := &session.Session{
		ID:            "sess-bundle",
		Agent:         "native-agent",
		Runtime:       runtimeinfo.NativeRuntime,
		MetadataDir:   metaDir,
		CreatedAt:     time.Now(),
		WorkspacePath: workDir,
	}
	if err := runtime.SaveState(sess, &runtime.State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: sess.ID,
		Agent:     sess.Agent,
		Model:     "openai/gpt-4o-mini",
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveEventLog(sess, &runtime.ParsedLog{
		Events: []runtime.Event{
			{Timestamp: time.Now().UTC(), Step: runtime.Step{Type: "text", Content: "hello"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.StderrLogPath(sess), []byte("stderr"), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := buildDebugReport(sess, 10, false)
	if err != nil {
		t.Fatal(err)
	}

	bundlePath := filepath.Join(t.TempDir(), "debug.tar.gz")
	if err := writeDebugBundle(bundlePath, report); err != nil {
		t.Fatal(err)
	}

	names := untarNames(t, bundlePath)
	for _, want := range []string{"summary.json", "state.json", "events.jsonl", "stderr.log"} {
		if !names[want] {
			t.Fatalf("expected %q in bundle, got %#v", want, names)
		}
	}
}

func TestResolveDebugSessionPrefixMatch(t *testing.T) {
	workspace := t.TempDir()
	withWorkingDir(t, workspace)

	fullID := "a46c28d5-1234-5678-9abc-def012345678"
	writeSessionsFile(t, workspace, []session.Session{
		{ID: fullID, Agent: "test-agent", CreatedAt: time.Now(), WorkspacePath: t.TempDir()},
		{ID: "b99f0000-aaaa-bbbb-cccc-dddddddddddd", Agent: "other-agent", CreatedAt: time.Now(), WorkspacePath: t.TempDir()},
	})

	// Exact match
	s, err := resolveDebugSession([]string{fullID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != fullID {
		t.Fatalf("expected %q, got %q", fullID, s.ID)
	}

	// Prefix match
	s, err = resolveDebugSession([]string{"a46c28d5"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != fullID {
		t.Fatalf("expected %q, got %q", fullID, s.ID)
	}

	// No match
	_, err = resolveDebugSession([]string{"ffffffff"}, false)
	if err == nil {
		t.Fatal("expected error for non-matching prefix")
	}
}

func TestResolveDebugSessionAmbiguousPrefix(t *testing.T) {
	workspace := t.TempDir()
	withWorkingDir(t, workspace)

	writeSessionsFile(t, workspace, []session.Session{
		{ID: "aaa11111-1111-1111-1111-111111111111", Agent: "a", CreatedAt: time.Now(), WorkspacePath: t.TempDir()},
		{ID: "aaa22222-2222-2222-2222-222222222222", Agent: "b", CreatedAt: time.Now(), WorkspacePath: t.TempDir()},
	})

	_, err := resolveDebugSession([]string{"aaa"}, false)
	if err == nil {
		t.Fatal("expected error for ambiguous prefix")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got: %s", err)
	}
}

func TestMostRecentDebugSessionUsesMetadataDirMtime(t *testing.T) {
	workspace := t.TempDir()
	withWorkingDir(t, workspace)
	if err := os.MkdirAll(filepath.Join(workspace, ".toc", "sessions", "older"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".toc", "sessions", "newer"), 0755); err != nil {
		t.Fatal(err)
	}
	writeSessionsFile(t, workspace, []session.Session{
		{ID: "older", Agent: "a", CreatedAt: time.Now().Add(-time.Hour), WorkspacePath: t.TempDir()},
		{ID: "newer", Agent: "b", CreatedAt: time.Now().Add(-2 * time.Hour), WorkspacePath: t.TempDir()},
	})
	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(filepath.Join(workspace, ".toc", "sessions", "older"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(workspace, ".toc", "sessions", "newer"), newTime, newTime); err != nil {
		t.Fatal(err)
	}

	sess, err := mostRecentDebugSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "newer" {
		t.Fatalf("session ID = %q", sess.ID)
	}
}

func TestFailedDebugSessionsFiltersAndSorts(t *testing.T) {
	workspace := t.TempDir()
	withWorkingDir(t, workspace)

	zombieDir := filepath.Join(workspace, ".toc", "sessions", "zombie")
	crashedDir := filepath.Join(workspace, ".toc", "sessions", "crashed")
	okDir := filepath.Join(workspace, ".toc", "sessions", "ok")
	for _, dir := range []string{zombieDir, crashedDir, okDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(zombieDir, "toc-pid.txt"), []byte("999999"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zombieDir, "state.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(&session.Session{ID: "crashed", MetadataDir: crashedDir, Runtime: runtimeinfo.NativeRuntime}, &runtime.State{
		Runtime:   runtimeinfo.NativeRuntime,
		SessionID: "crashed",
		Agent:     "agent-b",
		Status:    "crashed",
		LastError: "boom",
		CrashInfo: &runtime.CrashInfo{PanicMessage: "boom"},
	}); err != nil {
		t.Fatal(err)
	}
	writeSessionsFile(t, workspace, []session.Session{
		{
			ID:              "zombie",
			Agent:           "agent-a",
			Runtime:         runtimeinfo.NativeRuntime,
			MetadataDir:     zombieDir,
			CreatedAt:       time.Now().Add(-time.Minute),
			WorkspacePath:   zombieDir,
			Status:          session.StatusActive,
			ParentSessionID: "parent",
		},
		{
			ID:            "crashed",
			Agent:         "agent-b",
			Runtime:       runtimeinfo.NativeRuntime,
			MetadataDir:   crashedDir,
			CreatedAt:     time.Now(),
			WorkspacePath: okDir,
			Status:        session.StatusCompletedError,
		},
		{
			ID:            "ok",
			Agent:         "agent-c",
			Runtime:       runtimeinfo.NativeRuntime,
			MetadataDir:   okDir,
			CreatedAt:     time.Now().Add(-2 * time.Hour),
			WorkspacePath: okDir,
			Status:        session.StatusCompleted,
		},
	})

	items, err := failedDebugSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d", len(items))
	}
	if items[0].ID != "crashed" || items[1].ID != "zombie" {
		t.Fatalf("items = %#v", items)
	}
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatal(err)
		}
	})
}

func writeSessionsFile(t *testing.T, workspace string, sessions []session.Session) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, ".toc"), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(&session.SessionsFile{Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".toc", "sessions.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func untarNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()

	names := map[string]bool{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[hdr.Name] = true
		if hdr.Name == "summary.json" {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			var report debugReport
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
		}
	}
	return names
}
