package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	inspectpkg "github.com/tiny-oc/toc/internal/inspect"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	inspectCmd.Flags().Bool("json", false, "Output structured JSON")
	inspectCmd.Flags().Bool("body", false, "Include request and response bodies")
	inspectCmd.Flags().Bool("headers", false, "Include request and response headers")
	inspectCmd.Flags().Bool("last", false, "Resolve the most recent inspected session automatically")
	inspectCmd.Flags().Int("call", 0, "Show only one captured call by 1-based index")
	inspectCmd.ValidArgsFunction = completeSessionIDs
	rootCmd.AddCommand(inspectCmd)
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [session-id]",
	Short: "Inspect captured upstream LLM HTTP traffic for a session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		lastFlag, _ := cmd.Flags().GetBool("last")
		jsonFlag, _ := cmd.Flags().GetBool("json")
		bodyFlag, _ := cmd.Flags().GetBool("body")
		headersFlag, _ := cmd.Flags().GetBool("headers")
		callFlag, _ := cmd.Flags().GetInt("call")
		if callFlag < 0 {
			return fmt.Errorf("--call must be >= 0")
		}
		if lastFlag && len(args) > 0 {
			return fmt.Errorf("--last cannot be combined with a session ID")
		}

		sess, err := resolveInspectSession(args, lastFlag)
		if err != nil {
			return err
		}

		report, err := loadInspectReportForSession(sess)
		if err != nil {
			return err
		}

		if callFlag > 0 {
			if callFlag > len(report.Calls) {
				return fmt.Errorf("call %d out of range (session has %d captured call(s))", callFlag, len(report.Calls))
			}
			report.Calls = []inspectpkg.CallSummary{report.Calls[callFlag-1]}
		}

		if jsonFlag {
			return printInspectJSON(sess, report, bodyFlag, headersFlag)
		}
		printInspectHuman(sess, report, bodyFlag, headersFlag, callFlag > 0)
		return nil
	},
}

type inspectJSONOutput struct {
	Session     inspectSessionInfo       `json:"session"`
	CapturePath string                   `json:"capture_path"`
	Summary     inspectpkg.ReportSummary `json:"summary"`
	Calls       []inspectpkg.CallSummary `json:"calls"`
}

type inspectSessionInfo struct {
	ID        string `json:"id"`
	Agent     string `json:"agent"`
	Runtime   string `json:"runtime"`
	Status    string `json:"status"`
	Model     string `json:"model,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func resolveInspectSession(args []string, last bool) (*session.Session, error) {
	if last {
		return mostRecentInspectedSession()
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("session ID required unless --last is set")
	}
	return session.FindByIDOrPrefix(args[0])
}

func loadInspectReportForSession(sess *session.Session) (*inspectpkg.Report, error) {
	capturePath := runtime.InspectCapturePath(sess)
	if capturePath == "" {
		return nil, fmt.Errorf("session '%s' has no inspect capture path", sess.ID)
	}
	if _, err := os.Stat(capturePath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no inspect capture found for session '%s'. Re-run the session with --inspect", sess.ID)
		}
		return nil, err
	}
	return inspectpkg.LoadReport(capturePath)
}

func mostRecentInspectedSession() (*session.Session, error) {
	sf, err := session.Load()
	if err != nil {
		return nil, err
	}
	if len(sf.Sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	var best *session.Session
	var bestTime int64
	for i := range sf.Sessions {
		s := &sf.Sessions[i]
		capturePath := runtime.InspectCapturePath(s)
		info, err := os.Stat(capturePath)
		if err != nil {
			continue
		}
		mt := info.ModTime().UnixNano()
		if best == nil || mt > bestTime {
			best = s
			bestTime = mt
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no inspected sessions found")
	}
	return best, nil
}

func printInspectJSON(sess *session.Session, report *inspectpkg.Report, includeBodies, includeHeaders bool) error {
	calls := sanitizeInspectCalls(report.Calls, includeBodies, includeHeaders)
	model := ""
	if len(report.Summary.Models) > 0 {
		model = report.Summary.Models[0]
	}
	out := inspectJSONOutput{
		Session: inspectSessionInfo{
			ID:        sess.ID,
			Agent:     sess.Agent,
			Runtime:   sess.RuntimeName(),
			Status:    sess.ResolvedStatus(),
			Model:     model,
			CreatedAt: sess.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		},
		CapturePath: report.CapturePath,
		Summary:     report.Summary,
		Calls:       calls,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printInspectHuman(sess *session.Session, report *inspectpkg.Report, includeBodies, includeHeaders, singleCall bool) {
	fmt.Println()
	fmt.Printf("  %s  %s\n", ui.BoldCyan(sess.Agent), ui.Dim(shortID(sess.ID)))
	fmt.Printf("  %s  %s  %s\n", ui.Dim(sess.RuntimeName()), ui.Dim(sess.ResolvedStatus()), ui.Dim(fmt.Sprintf("%d call(s)", report.Summary.CallCount)))
	fmt.Printf("  %s %s\n", ui.Bold("Capture:"), ui.Dim(report.CapturePath))
	if len(report.Summary.Models) > 0 {
		fmt.Printf("  %s %s\n", ui.Bold("Models:"), ui.Dim(strings.Join(report.Summary.Models, ", ")))
	}
	if report.Summary.TotalTokens > 0 || report.Summary.InputTokens > 0 || report.Summary.OutputTokens > 0 {
		fmt.Printf("  %s in=%d out=%d total=%d\n", ui.Bold("Tokens:"), report.Summary.InputTokens, report.Summary.OutputTokens, report.Summary.TotalTokens)
	}
	fmt.Printf("  %s total=%dms avg=%dms errors=%d\n", ui.Bold("Latency:"), report.Summary.TotalDurationMS, report.Summary.AverageDurationMS, report.Summary.ErrorCount)
	fmt.Println()

	if !singleCall && len(report.Calls) > 0 {
		fmt.Printf("  %s\n", ui.Bold("Calls"))
	}
	for _, call := range report.Calls {
		printInspectCall(call, includeBodies, includeHeaders)
	}
}

func printInspectCall(call inspectpkg.CallSummary, includeBodies, includeHeaders bool) {
	status := fmt.Sprintf("%d", call.StatusCode)
	if call.StatusCode == 0 {
		status = "n/a"
	}
	if call.Error != "" && call.StatusCode == 0 {
		status = "error"
	}
	fmt.Printf("  #%d  %s %s  %s  %dms\n", call.Index, ui.Bold(status), ui.Dim(call.Method), ui.Dim(call.Path), call.DurationMS)
	if call.RequestModel != "" || call.ResponseModel != "" {
		model := inspectpkg.FirstNonEmpty(call.ResponseModel, call.RequestModel)
		fmt.Printf("    %s %s", ui.Bold("model:"), ui.Dim(model))
		if call.FinishReason != "" {
			fmt.Printf("   %s %s", ui.Bold("finish:"), ui.Dim(call.FinishReason))
		}
		fmt.Println()
	}
	if call.MessageCount > 0 || call.ToolCount > 0 {
		fmt.Printf("    %s %d   %s %d\n", ui.Bold("messages:"), call.MessageCount, ui.Bold("tools:"), call.ToolCount)
	}
	if call.InputTokens > 0 || call.OutputTokens > 0 || call.TotalTokens > 0 {
		fmt.Printf("    %s in=%d out=%d total=%d\n", ui.Bold("tokens:"), call.InputTokens, call.OutputTokens, call.TotalTokens)
	}
	if call.PromptPreview != "" {
		fmt.Printf("    %s %s\n", ui.Bold("prompt:"), ui.Dim(call.PromptPreview))
	}
	if call.Upstream != "" {
		fmt.Printf("    %s %s\n", ui.Bold("upstream:"), ui.Dim(call.Upstream))
	}
	if call.Error != "" {
		fmt.Printf("    %s %s\n", ui.Bold("error:"), ui.Red(call.Error))
	}
	if includeHeaders {
		printHeaderBlock("request headers", call.RequestHeaders)
		printHeaderBlock("response headers", call.ResponseHeaders)
	}
	if includeBodies {
		printBodyBlock("request body", call.RequestBody)
		printBodyBlock("response body", call.ResponseBody)
	}
	fmt.Println()
}

func printHeaderBlock(label string, headers map[string][]string) {
	if len(headers) == 0 {
		return
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("    %s\n", ui.Bold(label+":"))
	for _, k := range keys {
		fmt.Printf("      %s: %s\n", k, strings.Join(headers[k], ", "))
	}
}

func printBodyBlock(label, body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	fmt.Printf("    %s\n", ui.Bold(label+":"))
	fmt.Println(indentBlock(body, "      "))
}

func sanitizeInspectCalls(calls []inspectpkg.CallSummary, includeBodies, includeHeaders bool) []inspectpkg.CallSummary {
	out := make([]inspectpkg.CallSummary, len(calls))
	for i := range calls {
		out[i] = calls[i]
		if !includeBodies {
			out[i].RequestBody = ""
			out[i].ResponseBody = ""
		}
		if !includeHeaders {
			out[i].RequestHeaders = nil
			out[i].ResponseHeaders = nil
		}
	}
	return out
}

