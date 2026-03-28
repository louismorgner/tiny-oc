package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	inspectpkg "github.com/tiny-oc/toc/internal/inspect"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

const inspectLatencyDiffToleranceMS int64 = 50

func init() {
	inspectCompareCmd.Flags().Bool("json", false, "Output structured JSON")
	inspectCmd.AddCommand(inspectCompareCmd)
}

var inspectCompareCmd = &cobra.Command{
	Use:               "compare <session-a> <session-b>",
	Short:             "Compare captured upstream API traffic across two sessions",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeSessionIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		left, err := session.FindByIDOrPrefix(args[0])
		if err != nil {
			return err
		}
		right, err := session.FindByIDOrPrefix(args[1])
		if err != nil {
			return err
		}

		leftReport, err := loadInspectReportForSession(left)
		if err != nil {
			return err
		}
		rightReport, err := loadInspectReportForSession(right)
		if err != nil {
			return err
		}

		report := buildInspectCompareReport(left, leftReport, right, rightReport)
		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		printInspectCompareHuman(report)
		return nil
	},
}

type inspectCompareReport struct {
	Left  inspectCompareSession `json:"left"`
	Right inspectCompareSession `json:"right"`
	Delta inspectCompareDelta   `json:"delta"`
	Calls []inspectCallDiff     `json:"calls"`
}

type inspectCompareSession struct {
	ID          string                   `json:"id"`
	Agent       string                   `json:"agent"`
	Runtime     string                   `json:"runtime"`
	Status      string                   `json:"status"`
	CapturePath string                   `json:"capture_path"`
	Summary     inspectpkg.ReportSummary `json:"summary"`
}

type inspectCompareDelta struct {
	CallCountDelta         int      `json:"call_count_delta"`
	ErrorCountDelta        int      `json:"error_count_delta"`
	TotalDurationMSDelta   int64    `json:"total_duration_ms_delta"`
	AverageDurationMSDelta int64    `json:"average_duration_ms_delta"`
	InputTokensDelta       int64    `json:"input_tokens_delta"`
	OutputTokensDelta      int64    `json:"output_tokens_delta"`
	TotalTokensDelta       int64    `json:"total_tokens_delta"`
	ModelsOnlyLeft         []string `json:"models_only_left,omitempty"`
	ModelsOnlyRight        []string `json:"models_only_right,omitempty"`
	PathsOnlyLeft          []string `json:"paths_only_left,omitempty"`
	PathsOnlyRight         []string `json:"paths_only_right,omitempty"`
}

type inspectCallDiff struct {
	Index            int    `json:"index"`
	LeftPath         string `json:"left_path,omitempty"`
	RightPath        string `json:"right_path,omitempty"`
	LeftModel        string `json:"left_model,omitempty"`
	RightModel       string `json:"right_model,omitempty"`
	LeftStatusCode   int    `json:"left_status_code,omitempty"`
	RightStatusCode  int    `json:"right_status_code,omitempty"`
	LeftDurationMS   int64  `json:"left_duration_ms,omitempty"`
	RightDurationMS  int64  `json:"right_duration_ms,omitempty"`
	DurationDeltaMS  int64  `json:"duration_delta_ms,omitempty"`
	LeftTotalTokens  int64  `json:"left_total_tokens,omitempty"`
	RightTotalTokens int64  `json:"right_total_tokens,omitempty"`
	TokenDelta       int64  `json:"token_delta,omitempty"`
	LeftFinish       string `json:"left_finish,omitempty"`
	RightFinish      string `json:"right_finish,omitempty"`
	LeftPrompt       string `json:"left_prompt,omitempty"`
	RightPrompt      string `json:"right_prompt,omitempty"`
	SamePath         bool   `json:"same_path"`
	SameModel        bool   `json:"same_model"`
	SameStatus       bool   `json:"same_status"`
}

func buildInspectCompareReport(left *session.Session, leftReport *inspectpkg.Report, right *session.Session, rightReport *inspectpkg.Report) inspectCompareReport {
	report := inspectCompareReport{
		Left: inspectCompareSession{
			ID:          left.ID,
			Agent:       left.Agent,
			Runtime:     left.RuntimeName(),
			Status:      left.ResolvedStatus(),
			CapturePath: leftReport.CapturePath,
			Summary:     leftReport.Summary,
		},
		Right: inspectCompareSession{
			ID:          right.ID,
			Agent:       right.Agent,
			Runtime:     right.RuntimeName(),
			Status:      right.ResolvedStatus(),
			CapturePath: rightReport.CapturePath,
			Summary:     rightReport.Summary,
		},
		Delta: inspectCompareDelta{
			CallCountDelta:         rightReport.Summary.CallCount - leftReport.Summary.CallCount,
			ErrorCountDelta:        rightReport.Summary.ErrorCount - leftReport.Summary.ErrorCount,
			TotalDurationMSDelta:   rightReport.Summary.TotalDurationMS - leftReport.Summary.TotalDurationMS,
			AverageDurationMSDelta: rightReport.Summary.AverageDurationMS - leftReport.Summary.AverageDurationMS,
			InputTokensDelta:       rightReport.Summary.InputTokens - leftReport.Summary.InputTokens,
			OutputTokensDelta:      rightReport.Summary.OutputTokens - leftReport.Summary.OutputTokens,
			TotalTokensDelta:       rightReport.Summary.TotalTokens - leftReport.Summary.TotalTokens,
			ModelsOnlyLeft:         diffStrings(leftReport.Summary.Models, rightReport.Summary.Models),
			ModelsOnlyRight:        diffStrings(rightReport.Summary.Models, leftReport.Summary.Models),
			PathsOnlyLeft:          diffStrings(leftReport.Summary.Paths, rightReport.Summary.Paths),
			PathsOnlyRight:         diffStrings(rightReport.Summary.Paths, leftReport.Summary.Paths),
		},
	}

	maxCalls := len(leftReport.Calls)
	if len(rightReport.Calls) > maxCalls {
		maxCalls = len(rightReport.Calls)
	}
	for i := 0; i < maxCalls; i++ {
		diff := inspectCallDiff{Index: i + 1}
		if i < len(leftReport.Calls) {
			call := leftReport.Calls[i]
			diff.LeftPath = call.Path
			diff.LeftModel = inspectpkg.FirstNonEmpty(call.ResponseModel, call.RequestModel)
			diff.LeftStatusCode = call.StatusCode
			diff.LeftDurationMS = call.DurationMS
			diff.LeftTotalTokens = call.TotalTokens
			diff.LeftFinish = call.FinishReason
			diff.LeftPrompt = call.PromptPreview
		}
		if i < len(rightReport.Calls) {
			call := rightReport.Calls[i]
			diff.RightPath = call.Path
			diff.RightModel = inspectpkg.FirstNonEmpty(call.ResponseModel, call.RequestModel)
			diff.RightStatusCode = call.StatusCode
			diff.RightDurationMS = call.DurationMS
			diff.RightTotalTokens = call.TotalTokens
			diff.RightFinish = call.FinishReason
			diff.RightPrompt = call.PromptPreview
		}
		diff.DurationDeltaMS = diff.RightDurationMS - diff.LeftDurationMS
		diff.TokenDelta = diff.RightTotalTokens - diff.LeftTotalTokens
		diff.SamePath = diff.LeftPath == diff.RightPath
		diff.SameModel = diff.LeftModel == diff.RightModel
		diff.SameStatus = diff.LeftStatusCode == diff.RightStatusCode
		report.Calls = append(report.Calls, diff)
	}

	return report
}

func printInspectCompareHuman(report inspectCompareReport) {
	fmt.Println()
	fmt.Printf("  %s %s  %s %s\n", ui.Bold("A:"), ui.BoldCyan(report.Left.Agent), ui.Dim(shortID(report.Left.ID)), ui.Dim(report.Left.Runtime))
	fmt.Printf("  %s %s  %s %s\n", ui.Bold("B:"), ui.BoldCyan(report.Right.Agent), ui.Dim(shortID(report.Right.ID)), ui.Dim(report.Right.Runtime))
	fmt.Println()

	fmt.Printf("  %s\n", ui.Bold("Summary"))
	fmt.Printf("    calls:   %d -> %d (%s)\n", report.Left.Summary.CallCount, report.Right.Summary.CallCount, signedInt(report.Delta.CallCountDelta))
	fmt.Printf("    errors:  %d -> %d (%s)\n", report.Left.Summary.ErrorCount, report.Right.Summary.ErrorCount, signedInt(report.Delta.ErrorCountDelta))
	fmt.Printf("    latency: %dms -> %dms (%s)\n", report.Left.Summary.TotalDurationMS, report.Right.Summary.TotalDurationMS, signedInt64(report.Delta.TotalDurationMSDelta)+"ms")
	fmt.Printf("    avg:     %dms -> %dms (%s)\n", report.Left.Summary.AverageDurationMS, report.Right.Summary.AverageDurationMS, signedInt64(report.Delta.AverageDurationMSDelta)+"ms")
	fmt.Printf("    tokens:  %d -> %d (%s)\n", report.Left.Summary.TotalTokens, report.Right.Summary.TotalTokens, signedInt64(report.Delta.TotalTokensDelta))
	fmt.Printf("    input:   %d -> %d (%s)\n", report.Left.Summary.InputTokens, report.Right.Summary.InputTokens, signedInt64(report.Delta.InputTokensDelta))
	fmt.Printf("    output:  %d -> %d (%s)\n", report.Left.Summary.OutputTokens, report.Right.Summary.OutputTokens, signedInt64(report.Delta.OutputTokensDelta))
	if len(report.Delta.ModelsOnlyLeft) > 0 || len(report.Delta.ModelsOnlyRight) > 0 {
		fmt.Printf("    models:  A-only=%s  B-only=%s\n", formatList(report.Delta.ModelsOnlyLeft), formatList(report.Delta.ModelsOnlyRight))
	}
	if len(report.Delta.PathsOnlyLeft) > 0 || len(report.Delta.PathsOnlyRight) > 0 {
		fmt.Printf("    paths:   A-only=%s  B-only=%s\n", formatList(report.Delta.PathsOnlyLeft), formatList(report.Delta.PathsOnlyRight))
	}
	fmt.Println()

	fmt.Printf("  %s\n", ui.Bold("Calls"))
	for _, call := range report.Calls {
		status := inspectCallMatchLabel(call)
		fmt.Printf("    #%d  %s  path %s -> %s  model %s -> %s\n",
			call.Index,
			ui.Dim(status),
			compact(call.LeftPath),
			compact(call.RightPath),
			compact(call.LeftModel),
			compact(call.RightModel),
		)
		if call.LeftStatusCode != 0 || call.RightStatusCode != 0 {
			fmt.Printf("       status %s -> %s   latency %dms -> %dms (%s)   tokens %d -> %d (%s)\n",
				formatStatus(call.LeftStatusCode),
				formatStatus(call.RightStatusCode),
				call.LeftDurationMS,
				call.RightDurationMS,
				signedInt64(call.DurationDeltaMS)+"ms",
				call.LeftTotalTokens,
				call.RightTotalTokens,
				signedInt64(call.TokenDelta),
			)
		}
		if call.LeftFinish != call.RightFinish && (call.LeftFinish != "" || call.RightFinish != "") {
			fmt.Printf("       finish %s -> %s\n", compact(call.LeftFinish), compact(call.RightFinish))
		}
		if call.LeftPrompt != call.RightPrompt && (call.LeftPrompt != "" || call.RightPrompt != "") {
			fmt.Printf("       prompt %s -> %s\n", compact(call.LeftPrompt), compact(call.RightPrompt))
		}
	}
	fmt.Println()
}

func inspectCallMatchLabel(call inspectCallDiff) string {
	leftPopulated := call.LeftPath != "" || call.LeftStatusCode != 0 || call.LeftDurationMS != 0
	rightPopulated := call.RightPath != "" || call.RightStatusCode != 0 || call.RightDurationMS != 0
	if leftPopulated != rightPopulated {
		return "diff"
	}
	if !call.SamePath || !call.SameModel || !call.SameStatus || call.TokenDelta != 0 {
		return "diff"
	}
	if call.DurationDeltaMS > inspectLatencyDiffToleranceMS || call.DurationDeltaMS < -inspectLatencyDiffToleranceMS {
		return "diff"
	}
	return "same"
}

func diffStrings(left, right []string) []string {
	if len(left) == 0 {
		return nil
	}
	rightSet := make(map[string]bool, len(right))
	for _, item := range right {
		rightSet[item] = true
	}
	var diff []string
	for _, item := range left {
		if !rightSet[item] {
			diff = append(diff, item)
		}
	}
	return diff
}

func signedInt(v int) string {
	if v > 0 {
		return fmt.Sprintf("+%d", v)
	}
	return fmt.Sprintf("%d", v)
}

func signedInt64(v int64) string {
	if v > 0 {
		return fmt.Sprintf("+%d", v)
	}
	return fmt.Sprintf("%d", v)
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func compact(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	runes := []rune(s)
	if len(runes) > 48 {
		return string(runes[:45]) + "..."
	}
	return s
}

func formatStatus(code int) string {
	if code == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", code)
}
