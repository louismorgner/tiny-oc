package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tiny-oc/toc/internal/session"
)

func TestLoadPendingQuestionAndSubmitAnswer(t *testing.T) {
	metaDir := t.TempDir()
	sess := &session.Session{
		ID:          "child-question",
		MetadataDir: metaDir,
	}

	if err := os.WriteFile(filepath.Join(metaDir, "question.json"), []byte(`{"question":"Ship to staging?","timestamp":"2026-03-27T12:00:00Z","session_id":"child-question","agent":"implementer"}`), 0600); err != nil {
		t.Fatal(err)
	}

	question, err := LoadPendingQuestion(sess)
	if err != nil {
		t.Fatal(err)
	}
	if question == nil || question.Question != "Ship to staging?" {
		t.Fatalf("unexpected question: %#v", question)
	}

	if err := SubmitPendingQuestionAnswer(sess, "yes"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(metaDir, "question.json")); !os.IsNotExist(err) {
		t.Fatalf("question.json should be removed, got err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(metaDir, "answer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("answer.json should not be empty")
	}
}

func TestSubmitPendingQuestionAnswerResumesWaitingSession(t *testing.T) {
	workspace := t.TempDir()

	orig := questionPollTimeout
	questionPollTimeout = 3 * time.Second
	defer func() { questionPollTimeout = orig }()

	type result struct {
		exec toolExecution
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{nativeQuestion(nativeToolContext{
			Workspace: workspace,
			SessionID: "child-question",
			Agent:     "implementer",
		}, toolCall(t, "Question", map[string]interface{}{
			"question": "Should I keep the old flag?",
		}))}
	}()

	metaDir := MetadataDir(workspace, "child-question")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(metaDir, "question.json")); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	sess := &session.Session{
		ID:          "child-question",
		MetadataDir: metaDir,
	}
	if err := SubmitPendingQuestionAnswer(sess, "keep it"); err != nil {
		t.Fatal(err)
	}

	select {
	case result := <-ch:
		if result.exec.Step.Success == nil || !*result.exec.Step.Success {
			t.Fatalf("expected success, got %#v", result.exec)
		}
		if result.exec.Message != "keep it" {
			t.Fatalf("answer = %q", result.exec.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nativeQuestion did not resume after answer submission")
	}
}
