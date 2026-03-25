package cmd

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/tiny-oc/toc/internal/replay"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/usage"
)

func TestPrintReplayHuman_IncludesNativeFailureContext(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	r := &replay.Replay{
		SessionID:       "12345678-abcd",
		Agent:           "native-agent",
		Runtime:         runtimeinfo.NativeRuntime,
		Model:           "openai/gpt-4o-mini",
		Status:          session.StatusCancelled,
		ResumeCount:     2,
		CompactionCount: 1,
		LastError:       "session cancelled by parent session parent-1",
		DurationSecs:    12,
		Tokens:          usage.TokenUsage{InputTokens: 10, OutputTokens: 5},
		Steps: []runtime.Step{
			{Type: "compaction", Content: "Compacted 6 messages into toc-owned summary context."},
			{Type: "tool", Tool: "Write", Path: "result.txt", Success: testBoolPtr(true)},
			{Type: "error", Content: "session cancelled by parent session parent-1"},
		},
		FilesChanged: []string{"result.txt"},
		ToolCount:    1,
		ErrorCount:   1,
	}

	output := captureStdout(t, func() {
		if err := printReplayHuman(r, false, false, false, true); err != nil {
			t.Fatal(err)
		}
	})

	for _, want := range []string{"toc-native", "openai/gpt-4o-mini", "cancelled", "Resumes: 2", "Compactions: 1", "[compact]", "Last error: session cancelled by parent session parent-1"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output: %q", want, output)
		}
	}
}

func testBoolPtr(v bool) *bool {
	return &v
}
