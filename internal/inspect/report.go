package inspect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type Report struct {
	CapturePath string        `json:"capture_path"`
	Calls       []CallSummary `json:"calls"`
	Summary     ReportSummary `json:"summary"`
}

type ReportSummary struct {
	CallCount         int      `json:"call_count"`
	ErrorCount        int      `json:"error_count"`
	TotalDurationMS   int64    `json:"total_duration_ms"`
	InputTokens       int64    `json:"input_tokens,omitempty"`
	OutputTokens      int64    `json:"output_tokens,omitempty"`
	TotalTokens       int64    `json:"total_tokens,omitempty"`
	Models            []string `json:"models,omitempty"`
	Paths             []string `json:"paths,omitempty"`
	AverageDurationMS int64    `json:"average_duration_ms,omitempty"`
}

type CallSummary struct {
	Index           int                 `json:"index"`
	Timestamp       time.Time           `json:"timestamp"`
	Method          string              `json:"method"`
	Path            string              `json:"path"`
	Upstream        string              `json:"upstream_url"`
	StatusCode      int                 `json:"status_code,omitempty"`
	DurationMS      int64               `json:"duration_ms"`
	RequestModel    string              `json:"request_model,omitempty"`
	ResponseModel   string              `json:"response_model,omitempty"`
	MessageCount    int                 `json:"message_count,omitempty"`
	ToolCount       int                 `json:"tool_count,omitempty"`
	PromptPreview   string              `json:"prompt_preview,omitempty"`
	FinishReason    string              `json:"finish_reason,omitempty"`
	InputTokens     int64               `json:"input_tokens,omitempty"`
	OutputTokens    int64               `json:"output_tokens,omitempty"`
	TotalTokens     int64               `json:"total_tokens,omitempty"`
	Error           string              `json:"error,omitempty"`
	RequestBody     string              `json:"request_body,omitempty"`
	ResponseBody    string              `json:"response_body,omitempty"`
	RequestHeaders  map[string][]string `json:"request_headers,omitempty"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
}

func LoadReport(capturePath string) (*Report, error) {
	f, err := os.Open(capturePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	report := &Report{CapturePath: capturePath}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)

	models := map[string]bool{}
	paths := map[string]bool{}
	index := 0
	for scanner.Scan() {
		var entry CaptureEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode capture line %d: %w", index+1, err)
		}
		index++
		call := summarizeEntry(index, entry)
		report.Calls = append(report.Calls, call)
		report.Summary.CallCount++
		report.Summary.TotalDurationMS += call.DurationMS
		report.Summary.InputTokens += call.InputTokens
		report.Summary.OutputTokens += call.OutputTokens
		report.Summary.TotalTokens += call.TotalTokens
		if call.StatusCode >= 400 || call.Error != "" {
			report.Summary.ErrorCount++
		}
		if call.RequestModel != "" {
			models[call.RequestModel] = true
		}
		if call.ResponseModel != "" {
			models[call.ResponseModel] = true
		}
		if call.Path != "" {
			paths[call.Path] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	report.Summary.Models = sortedSet(models)
	report.Summary.Paths = sortedSet(paths)
	if report.Summary.CallCount > 0 {
		report.Summary.AverageDurationMS = report.Summary.TotalDurationMS / int64(report.Summary.CallCount)
	}
	return report, nil
}

func summarizeEntry(index int, entry CaptureEntry) CallSummary {
	call := CallSummary{
		Index:          index,
		Timestamp:      entry.Timestamp,
		Method:         entry.Method,
		Path:           entry.Path,
		Upstream:       entry.Upstream,
		DurationMS:     entry.DurationMS,
		Error:          entry.Error,
		RequestBody:    entry.Request.Body,
		ResponseBody:   bodyFromResponse(entry.Response),
		RequestHeaders: entry.Request.Headers,
	}
	if entry.Response != nil {
		call.StatusCode = entry.Response.StatusCode
		call.ResponseHeaders = entry.Response.Headers
	}

	var req map[string]interface{}
	if json.Unmarshal([]byte(entry.Request.Body), &req) == nil {
		call.RequestModel = stringField(req["model"])
		call.MessageCount = len(interfaceSlice(req["messages"]))
		call.ToolCount = len(interfaceSlice(req["tools"]))
		call.PromptPreview = truncate(flattenPromptPreview(req["messages"]), 140)
	}

	if entry.Response != nil {
		var resp map[string]interface{}
		if json.Unmarshal([]byte(entry.Response.Body), &resp) == nil {
			call.ResponseModel = stringField(resp["model"])
			call.FinishReason = FirstNonEmpty(
				stringField(resp["stop_reason"]),
				choiceFinishReason(resp["choices"]),
			)
			call.InputTokens, call.OutputTokens, call.TotalTokens = extractUsage(resp["usage"])
		} else {
			// Response body may be SSE (streaming). Parse the last data line
			// that contains usage/model/finish info.
			parseSSEResponseFields(&call, entry.Response.Body)
		}
	}

	return call
}

func parseSSEResponseFields(call *CallSummary, body string) {
	// Walk SSE data lines in reverse to find the last ones with JSON payloads
	// containing usage, model, or finish_reason/stop_reason.
	lines := strings.Split(body, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var obj map[string]interface{}
		if json.Unmarshal([]byte(payload), &obj) != nil {
			continue
		}
		if call.ResponseModel == "" {
			call.ResponseModel = stringField(obj["model"])
		}
		if call.FinishReason == "" {
			call.FinishReason = FirstNonEmpty(
				stringField(obj["stop_reason"]),
				choiceFinishReason(obj["choices"]),
			)
		}
		if call.InputTokens == 0 && call.OutputTokens == 0 && call.TotalTokens == 0 {
			call.InputTokens, call.OutputTokens, call.TotalTokens = extractUsage(obj["usage"])
		}
		if call.ResponseModel != "" && call.FinishReason != "" && (call.InputTokens > 0 || call.OutputTokens > 0) {
			break
		}
	}
}

func bodyFromResponse(resp *CaptureResponse) string {
	if resp == nil {
		return ""
	}
	return resp.Body
}

func extractUsage(raw interface{}) (int64, int64, int64) {
	usage, ok := raw.(map[string]interface{})
	if !ok {
		return 0, 0, 0
	}
	input := int64Field(usage["prompt_tokens"])
	output := int64Field(usage["completion_tokens"])
	total := int64Field(usage["total_tokens"])
	if input == 0 && output == 0 && total == 0 {
		input = int64Field(usage["input_tokens"])
		// Anthropic prompt caching: add cache tokens to get the full effective
		// input context size. Cache reads are cheap but still represent context
		// that the model processes; cache creation tokens are the initial write.
		input += int64Field(usage["cache_read_input_tokens"])
		input += int64Field(usage["cache_creation_input_tokens"])
		output = int64Field(usage["output_tokens"])
		total = input + output
	}
	if total == 0 {
		total = input + output
	}
	return input, output, total
}

func choiceFinishReason(raw interface{}) string {
	choices := interfaceSlice(raw)
	if len(choices) == 0 {
		return ""
	}
	first, ok := choices[0].(map[string]interface{})
	if !ok {
		return ""
	}
	return stringField(first["finish_reason"])
}

func flattenPromptPreview(raw interface{}) string {
	messages := interfaceSlice(raw)
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]interface{})
		if !ok || stringField(msg["role"]) != "user" {
			continue
		}
		if text := flattenContent(msg["content"]); text != "" {
			return text
		}
	}
	return ""
}

func flattenContent(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		var parts []string
		for _, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			text := strings.TrimSpace(stringField(m["text"]))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]interface{}:
		return strings.TrimSpace(stringField(v["text"]))
	default:
		return ""
	}
}

func interfaceSlice(raw interface{}) []interface{} {
	if raw == nil {
		return nil
	}
	if v, ok := raw.([]interface{}); ok {
		return v
	}
	return nil
}

func stringField(raw interface{}) string {
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}

func int64Field(raw interface{}) int64 {
	switch v := raw.(type) {
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// FirstNonEmpty returns the first non-blank string from values.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func sortedSet(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for v := range values {
		items = append(items, v)
	}
	sort.Strings(items)
	return items
}
