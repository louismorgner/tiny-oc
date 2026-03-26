package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

func TestPrintOutput_WithMetaIncludesNativeState(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	sess := &session.Session{
		ID:          "sess-output",
		Agent:       "native-agent",
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: t.TempDir(),
	}
	if err := runtime.SaveState(sess, &runtime.State{
		Runtime:         runtimeinfo.NativeRuntime,
		SessionID:       sess.ID,
		Agent:           sess.Agent,
		Model:           "openai/gpt-4o-mini",
		ResumeCount:     1,
		CompactionCount: 2,
		LastError:       "cancelled by parent",
		Usage: runtime.TokenUsageSnapshot{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := printOutput(false, true, sess, session.StatusCancelled, []byte("partial output")); err != nil {
			t.Fatal(err)
		}
	})

	for _, want := range []string{"Session:", "Runtime:", "Model:", "Resumes:", "Compactions:", "Tokens:", "Last error:", "partial output"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output: %q", want, output)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}
