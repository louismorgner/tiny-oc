package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
)

func TestShowPendingQuestionDisplaysAnswerHint(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	workspace := t.TempDir()
	withWorkingDir(t, workspace)
	writeWorkspaceConfig(t, workspace)

	metadataDir := filepath.Join(workspace, ".toc", "sessions", "child-question")
	writeSessionsFile(t, workspace, []session.Session{
		{
			ID:              "child-question",
			Agent:           "implementer",
			MetadataDir:     metadataDir,
			CreatedAt:       time.Now(),
			WorkspacePath:   t.TempDir(),
			Status:          session.StatusActive,
			ParentSessionID: "parent-question",
		},
	})
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataDir, "question.json"), []byte(`{"question":"Use the new endpoint?","timestamp":"2026-03-27T12:00:00Z","session_id":"child-question","agent":"implementer"}`), 0600); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := showPendingQuestion("child-question", false); err != nil {
			t.Fatal(err)
		}
	})

	for _, want := range []string{"Use the new endpoint?", "toc answer child-question --text"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output: %q", want, output)
		}
	}
}

func TestAnswerCommandWritesAnswerAndClearsQuestion(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	workspace := t.TempDir()
	withWorkingDir(t, workspace)
	writeWorkspaceConfig(t, workspace)

	metadataDir := filepath.Join(workspace, ".toc", "sessions", "child-question")
	writeSessionsFile(t, workspace, []session.Session{
		{
			ID:              "child-question",
			Agent:           "implementer",
			MetadataDir:     metadataDir,
			CreatedAt:       time.Now(),
			WorkspacePath:   t.TempDir(),
			Status:          session.StatusActive,
			ParentSessionID: "parent-question",
		},
	})
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataDir, "question.json"), []byte(`{"question":"Use the new endpoint?","timestamp":"2026-03-27T12:00:00Z","session_id":"child-question","agent":"implementer"}`), 0600); err != nil {
		t.Fatal(err)
	}

	answerText = "yes"
	t.Cleanup(func() { answerText = "" })

	output := captureStdout(t, func() {
		if err := answerCmd.RunE(answerCmd, []string{"child-question"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Submitted answer") {
		t.Fatalf("expected success output: %q", output)
	}

	if _, err := os.Stat(filepath.Join(metadataDir, "question.json")); !os.IsNotExist(err) {
		t.Fatalf("question.json should be removed, got err=%v", err)
	}
	answerData, err := os.ReadFile(filepath.Join(metadataDir, "answer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(answerData), `"answer":"yes"`) {
		t.Fatalf("unexpected answer.json: %s", answerData)
	}
}

func TestListPendingQuestionsShowsWorkspaceQuestions(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	workspace := t.TempDir()
	withWorkingDir(t, workspace)
	writeWorkspaceConfig(t, workspace)

	writeSessionsFile(t, workspace, []session.Session{
		{
			ID:            "child-one",
			Agent:         "implementer",
			MetadataDir:   filepath.Join(workspace, ".toc", "sessions", "child-one"),
			CreatedAt:     time.Now(),
			WorkspacePath: t.TempDir(),
			Status:        session.StatusActive,
		},
		{
			ID:            "child-two",
			Agent:         "reviewer",
			MetadataDir:   filepath.Join(workspace, ".toc", "sessions", "child-two"),
			CreatedAt:     time.Now(),
			WorkspacePath: t.TempDir(),
			Status:        session.StatusActive,
		},
	})
	for _, sess := range []struct {
		id       string
		question string
	}{
		{id: "child-one", question: "Deploy now?"},
		{id: "child-two", question: "Run the migration?"},
	} {
		metaDir := runtime.MetadataDir(workspace, sess.id)
		if err := os.MkdirAll(metaDir, 0755); err != nil {
			t.Fatal(err)
		}
		payload := `{"question":"` + sess.question + `","timestamp":"2026-03-27T12:00:00Z","session_id":"` + sess.id + `","agent":"tester"}`
		if err := os.WriteFile(filepath.Join(metaDir, "question.json"), []byte(payload), 0600); err != nil {
			t.Fatal(err)
		}
	}

	output := captureStdout(t, func() {
		if err := listPendingQuestions(false); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"child-on", "child-tw", "Deploy now?", "Run the migration?"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output: %q", want, output)
		}
	}
}

func writeWorkspaceConfig(t *testing.T, workspace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, ".toc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".toc", "config.yaml"), []byte("name: test-workspace\n"), 0644); err != nil {
		t.Fatal(err)
	}
}
