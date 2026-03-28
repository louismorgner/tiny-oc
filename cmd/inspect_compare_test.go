package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fatih/color"
	inspectpkg "github.com/tiny-oc/toc/internal/inspect"
	"github.com/tiny-oc/toc/internal/session"
)

func TestBuildInspectCompareReport(t *testing.T) {
	left := &session.Session{ID: "left-session", Agent: "claude", Runtime: "claude-code"}
	right := &session.Session{ID: "right-session", Agent: "native", Runtime: "toc-native"}

	leftReport := &inspectpkg.Report{
		CapturePath: "/tmp/left.jsonl",
		Summary: inspectpkg.ReportSummary{
			CallCount:         2,
			ErrorCount:        0,
			TotalDurationMS:   300,
			AverageDurationMS: 150,
			InputTokens:       100,
			OutputTokens:      40,
			TotalTokens:       140,
			Models:            []string{"claude-sonnet-4"},
			Paths:             []string{"/v1/messages"},
		},
		Calls: []inspectpkg.CallSummary{
			{Index: 1, Path: "/v1/messages", RequestModel: "claude-sonnet-4", StatusCode: 200, DurationMS: 120, TotalTokens: 60, PromptPreview: "analyze repo"},
			{Index: 2, Path: "/v1/messages", RequestModel: "claude-sonnet-4", StatusCode: 200, DurationMS: 180, TotalTokens: 80, FinishReason: "stop"},
		},
	}
	rightReport := &inspectpkg.Report{
		CapturePath: "/tmp/right.jsonl",
		Summary: inspectpkg.ReportSummary{
			CallCount:         3,
			ErrorCount:        1,
			TotalDurationMS:   390,
			AverageDurationMS: 130,
			InputTokens:       120,
			OutputTokens:      55,
			TotalTokens:       175,
			Models:            []string{"openai/gpt-4o-mini"},
			Paths:             []string{"/chat/completions"},
		},
		Calls: []inspectpkg.CallSummary{
			{Index: 1, Path: "/chat/completions", RequestModel: "openai/gpt-4o-mini", StatusCode: 200, DurationMS: 100, TotalTokens: 50, PromptPreview: "analyze repo"},
			{Index: 2, Path: "/chat/completions", RequestModel: "openai/gpt-4o-mini", StatusCode: 500, DurationMS: 140, TotalTokens: 60, FinishReason: "error"},
			{Index: 3, Path: "/chat/completions", RequestModel: "openai/gpt-4o-mini", StatusCode: 200, DurationMS: 150, TotalTokens: 65},
		},
	}

	report := buildInspectCompareReport(left, leftReport, right, rightReport)
	if report.Delta.CallCountDelta != 1 {
		t.Fatalf("CallCountDelta = %d", report.Delta.CallCountDelta)
	}
	if report.Delta.TotalTokensDelta != 35 {
		t.Fatalf("TotalTokensDelta = %d", report.Delta.TotalTokensDelta)
	}
	if len(report.Delta.ModelsOnlyLeft) != 1 || report.Delta.ModelsOnlyLeft[0] != "claude-sonnet-4" {
		t.Fatalf("ModelsOnlyLeft = %#v", report.Delta.ModelsOnlyLeft)
	}
	if len(report.Calls) != 3 {
		t.Fatalf("expected 3 call diffs, got %d", len(report.Calls))
	}
	if report.Calls[1].RightStatusCode != 500 {
		t.Fatalf("second call diff = %#v", report.Calls[1])
	}
}

func TestPrintInspectCompareHuman(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	report := inspectCompareReport{
		Left: inspectCompareSession{
			ID:      "left-session",
			Agent:   "claude",
			Runtime: "claude-code",
			Summary: inspectpkg.ReportSummary{CallCount: 2, TotalTokens: 140, InputTokens: 100, OutputTokens: 40, TotalDurationMS: 300, AverageDurationMS: 150},
		},
		Right: inspectCompareSession{
			ID:      "right-session",
			Agent:   "native",
			Runtime: "toc-native",
			Summary: inspectpkg.ReportSummary{CallCount: 3, TotalTokens: 175, InputTokens: 120, OutputTokens: 55, TotalDurationMS: 390, AverageDurationMS: 130, ErrorCount: 1},
		},
		Delta: inspectCompareDelta{
			CallCountDelta:         1,
			TotalTokensDelta:       35,
			InputTokensDelta:       20,
			OutputTokensDelta:      15,
			TotalDurationMSDelta:   90,
			AverageDurationMSDelta: -20,
			PathsOnlyLeft:          []string{"/v1/messages"},
			PathsOnlyRight:         []string{"/chat/completions"},
		},
		Calls: []inspectCallDiff{
			{Index: 1, LeftPath: "/v1/messages", RightPath: "/chat/completions", LeftModel: "claude-sonnet-4", RightModel: "openai/gpt-4o-mini", LeftStatusCode: 200, RightStatusCode: 200, LeftDurationMS: 120, RightDurationMS: 100, DurationDeltaMS: -20, LeftTotalTokens: 60, RightTotalTokens: 50, TokenDelta: -10},
		},
	}

	out := captureStdout(t, func() {
		printInspectCompareHuman(report)
	})
	for _, want := range []string{"Summary", "calls:", "paths:", "Calls", "#1", "/v1/messages", "/chat/completions"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output: %q", want, out)
		}
	}
}

func TestInspectCompareJSON(t *testing.T) {
	report := inspectCompareReport{
		Delta: inspectCompareDelta{CallCountDelta: 1},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"call_count_delta":1`) {
		t.Fatalf("unexpected json: %s", data)
	}
}

func TestInspectCallMatchLabel_ToleratesSmallLatencyDelta(t *testing.T) {
	call := inspectCallDiff{
		SamePath:        true,
		SameModel:       true,
		SameStatus:      true,
		DurationDeltaMS: 25,
	}
	if got := inspectCallMatchLabel(call); got != "same" {
		t.Fatalf("inspectCallMatchLabel() = %q, want same", got)
	}

	call.DurationDeltaMS = 75
	if got := inspectCallMatchLabel(call); got != "diff" {
		t.Fatalf("inspectCallMatchLabel() = %q, want diff", got)
	}
}
