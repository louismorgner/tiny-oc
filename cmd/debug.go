package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	goRuntime "runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	iruntime "github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
	"github.com/tiny-oc/toc/internal/usage"
)

func init() {
	debugCmd.Flags().Bool("json", false, "Output structured JSON")
	debugCmd.Flags().Bool("full", false, "Include full state and event history")
	debugCmd.Flags().String("bundle", "", "Write a diagnostic tar.gz bundle")
	debugCmd.Flags().Int("events", 50, "Number of recent events to include")
	debugCmd.Flags().Bool("last", false, "Resolve the most recent session automatically")
	debugCmd.Flags().Bool("failed", false, "List failed or zombie sessions")
	debugCmd.ValidArgsFunction = completeSessionIDs
	rootCmd.AddCommand(debugCmd)
}

var debugCmd = &cobra.Command{
	Use:   "debug [session-id]",
	Short: "Collect diagnostic data for a session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		fullFlag, _ := cmd.Flags().GetBool("full")
		lastFlag, _ := cmd.Flags().GetBool("last")
		failedFlag, _ := cmd.Flags().GetBool("failed")
		bundlePath, _ := cmd.Flags().GetString("bundle")
		eventLimit, _ := cmd.Flags().GetInt("events")
		if eventLimit < 0 {
			return fmt.Errorf("--events must be >= 0")
		}
		if lastFlag && len(args) > 0 {
			return fmt.Errorf("--last cannot be combined with a session ID")
		}
		if failedFlag && (lastFlag || len(args) > 0 || bundlePath != "") {
			return fmt.Errorf("--failed cannot be combined with a session ID, --last, or --bundle")
		}

		if failedFlag {
			sessions, err := failedDebugSessions()
			if err != nil {
				return err
			}
			if jsonFlag {
				data, err := json.MarshalIndent(sessions, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(data))
				return nil
			}
			printFailedDebugSessions(sessions)
			return nil
		}

		sess, err := resolveDebugSession(args, lastFlag)
		if err != nil {
			return err
		}
		report, err := buildDebugReport(sess, eventLimit, fullFlag)
		if err != nil {
			return err
		}
		if bundlePath != "" {
			if err := writeDebugBundle(bundlePath, report); err != nil {
				return err
			}
		}
		if jsonFlag {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		printDebugReport(report, fullFlag)
		if bundlePath != "" {
			fmt.Println()
			ui.Success("Wrote diagnostic bundle: %s", bundlePath)
		}
		return nil
	},
}

type debugReport struct {
	Session       debugSessionInfo      `json:"session"`
	State         debugStateInfo        `json:"state"`
	Timeline      debugTimelineInfo     `json:"timeline"`
	Process       debugProcessInfo      `json:"process"`
	Usage         debugUsageInfo        `json:"usage"`
	System        debugSystemInfo       `json:"system"`
	Output        *debugArtifact        `json:"output,omitempty"`
	Stderr        *debugArtifact        `json:"stderr,omitempty"`
	FullState     *iruntime.State       `json:"full_state,omitempty"`
	FullEvents    []iruntime.Event      `json:"full_events,omitempty"`
	MetadataFiles []debugBundleArtifact `json:"metadata_files,omitempty"`
}

type debugSessionInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name,omitempty"`
	Agent       string    `json:"agent"`
	Runtime     string    `json:"runtime"`
	CreatedAt   time.Time `json:"created_at"`
	StartTime   time.Time `json:"start_time,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Duration    string    `json:"duration,omitempty"`
	Status      string    `json:"status"`
	ExitReason  string    `json:"exit_reason,omitempty"`
	Workspace   string    `json:"workspace"`
	MetadataDir string    `json:"metadata_dir,omitempty"`
}

type debugStateInfo struct {
	RuntimeStatus    string                   `json:"runtime_status,omitempty"`
	Model            string                   `json:"model,omitempty"`
	LastError        string                   `json:"last_error,omitempty"`
	PendingTurn      *iruntime.TurnCheckpoint `json:"pending_turn,omitempty"`
	PendingTurnLabel string                   `json:"pending_turn_label,omitempty"`
	RecoveryCount    int                      `json:"recovery_count,omitempty"`
	ResumeCount      int                      `json:"resume_count,omitempty"`
	CompactionCount  int                      `json:"compaction_count,omitempty"`
	CrashInfo        *iruntime.CrashInfo      `json:"crash_info,omitempty"`
}

type debugTimelineInfo struct {
	TotalEvents    int              `json:"total_events"`
	EventTypes     map[string]int   `json:"event_types,omitempty"`
	LastToolCall   string           `json:"last_tool_call,omitempty"`
	LastErrorEvent *iruntime.Event  `json:"last_error_event,omitempty"`
	RecentEvents   []iruntime.Event `json:"recent_events,omitempty"`
}

type debugProcessInfo struct {
	PID      *int `json:"pid,omitempty"`
	Alive    bool `json:"alive"`
	Zombie   bool `json:"zombie"`
	ExitCode *int `json:"exit_code,omitempty"`
}

type debugUsageInfo struct {
	InputTokens  int64    `json:"input_tokens,omitempty"`
	OutputTokens int64    `json:"output_tokens,omitempty"`
	CacheRead    int64    `json:"cache_read,omitempty"`
	CacheCreate  int64    `json:"cache_create,omitempty"`
	TotalTokens  int64    `json:"total_tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
}

type debugSystemInfo struct {
	Version string `json:"toc_version"`
	Go      string `json:"go_version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

type debugArtifact struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type debugFailedSession struct {
	ID          string    `json:"id"`
	Agent       string    `json:"agent"`
	Name        string    `json:"name,omitempty"`
	Status      string    `json:"status"`
	Runtime     string    `json:"runtime"`
	CreatedAt   time.Time `json:"created_at"`
	LastError   string    `json:"last_error,omitempty"`
	MetadataDir string    `json:"metadata_dir,omitempty"`
}

type debugBundleArtifact struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func resolveDebugSession(args []string, last bool) (*session.Session, error) {
	if last {
		return mostRecentDebugSession()
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("session ID required unless --last or --failed is set")
	}
	return session.FindByIDOrPrefix(args[0])
}

func mostRecentDebugSession() (*session.Session, error) {
	sf, err := session.Load()
	if err != nil {
		return nil, err
	}
	if len(sf.Sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	byID := make(map[string]session.Session, len(sf.Sessions))
	for _, s := range sf.Sessions {
		byID[s.ID] = s
	}

	entries, err := os.ReadDir(config.SessionsDir())
	if err == nil {
		type candidate struct {
			session session.Session
			modTime time.Time
		}
		var candidates []candidate
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			s, ok := byID[entry.Name()]
			if !ok {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{session: s, modTime: info.ModTime()})
		}
		if len(candidates) > 0 {
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].modTime.After(candidates[j].modTime)
			})
			return &candidates[0].session, nil
		}
	}

	sort.Slice(sf.Sessions, func(i, j int) bool {
		return sf.Sessions[i].CreatedAt.After(sf.Sessions[j].CreatedAt)
	})
	return &sf.Sessions[0], nil
}

func failedDebugSessions() ([]debugFailedSession, error) {
	sf, err := session.Load()
	if err != nil {
		return nil, err
	}

	var result []debugFailedSession
	for _, s := range sf.Sessions {
		state, _ := loadDebugState(&s)
		if !isDebugFailure(&s, state) {
			continue
		}
		result = append(result, debugFailedSession{
			ID:          s.ID,
			Agent:       s.Agent,
			Name:        s.Name,
			Status:      s.ResolvedStatus(),
			Runtime:     s.RuntimeName(),
			CreatedAt:   s.CreatedAt,
			LastError:   stateField(state, func(v *iruntime.State) string { return v.LastError }),
			MetadataDir: s.MetadataDirPath(),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func isDebugFailure(s *session.Session, state *iruntime.State) bool {
	status := s.ResolvedStatus()
	if status == session.StatusCompletedError || status == session.StatusZombie {
		return true
	}
	if state == nil {
		return false
	}
	if state.CrashInfo != nil {
		return true
	}
	return state.Status == "failed" || state.Status == "crashed"
}

func buildDebugReport(s *session.Session, eventLimit int, full bool) (*debugReport, error) {
	if err := iruntime.PreserveCrashInfo(s); err != nil {
		return nil, err
	}

	state, err := loadDebugState(s)
	if err != nil {
		return nil, err
	}
	events, err := loadDebugEvents(s)
	if err != nil {
		return nil, err
	}

	startTime := s.CreatedAt
	if state != nil && !state.CreatedAt.IsZero() {
		startTime = state.CreatedAt
	}
	updatedAt := startTime
	if state != nil && !state.UpdatedAt.IsZero() {
		updatedAt = state.UpdatedAt
	}
	if updatedAt.IsZero() {
		updatedAt = s.CreatedAt
	}
	if updatedAt.Before(startTime) {
		updatedAt = startTime
	}

	status := s.ResolvedStatus()
	report := &debugReport{
		Session: debugSessionInfo{
			ID:          s.ID,
			Name:        s.Name,
			Agent:       s.Agent,
			Runtime:     s.RuntimeName(),
			CreatedAt:   s.CreatedAt,
			StartTime:   startTime,
			UpdatedAt:   updatedAt,
			Duration:    updatedAt.Sub(startTime).Round(time.Second).String(),
			Status:      status,
			ExitReason:  debugExitReason(s, state, status),
			Workspace:   s.WorkspacePath,
			MetadataDir: s.MetadataDirPath(),
		},
		Process: debugProcessDetails(s, status),
		Usage:   debugUsageDetails(s, state),
		System: debugSystemInfo{
			Version: version,
			Go:      goRuntime.Version(),
			OS:      goRuntime.GOOS,
			Arch:    goRuntime.GOARCH,
		},
	}

	if state != nil {
		report.State = debugStateInfo{
			RuntimeStatus:    state.Status,
			Model:            state.Model,
			LastError:        state.LastError,
			PendingTurn:      state.PendingTurn,
			PendingTurnLabel: iruntime.DescribePendingTurn(state.PendingTurn),
			RecoveryCount:    state.RecoveryCount,
			ResumeCount:      state.ResumeCount,
			CompactionCount:  state.CompactionCount,
			CrashInfo:        state.CrashInfo,
		}
	}

	report.Timeline = debugTimelineDetails(state, events, eventLimit, full)
	report.Output = readDebugArtifact(s.WorkspacePath+"/toc-output.txt", s.WorkspacePath+"/toc-output.txt.tmp")
	report.Stderr = readDebugArtifact(iruntime.StderrLogPath(s))
	report.MetadataFiles = debugMetadataArtifacts(s)

	if full {
		report.FullState = state
		report.FullEvents = events
	}

	return report, nil
}

func loadDebugState(s *session.Session) (*iruntime.State, error) {
	state, err := iruntime.LoadState(s)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return state, nil
}

func loadDebugEvents(s *session.Session) ([]iruntime.Event, error) {
	parsed, err := iruntime.LoadEventLog(s)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parsed.Events, nil
}

func debugTimelineDetails(state *iruntime.State, events []iruntime.Event, eventLimit int, full bool) debugTimelineInfo {
	typeCounts := make(map[string]int)
	var lastError *iruntime.Event
	lastTool := ""
	for i := range events {
		ev := events[i]
		typeCounts[ev.Step.Type]++
		if ev.Step.Type == "error" || ev.Step.Type == "crash" {
			evCopy := ev
			lastError = &evCopy
		}
		if ev.Step.Tool != "" {
			lastTool = ev.Step.Tool
		}
	}
	if lastTool == "" {
		lastTool = iruntime.LastToolCall(state)
	}

	recent := events
	if !full && eventLimit >= 0 && len(events) > eventLimit {
		recent = events[len(events)-eventLimit:]
	}

	return debugTimelineInfo{
		TotalEvents:    len(events),
		EventTypes:     typeCounts,
		LastToolCall:   lastTool,
		LastErrorEvent: lastError,
		RecentEvents:   recent,
	}
}

func debugProcessDetails(s *session.Session, status string) debugProcessInfo {
	info := debugProcessInfo{Zombie: status == session.StatusZombie}
	if pid, err := s.ReadPID(); err == nil {
		info.PID = &pid
		info.Alive = syscall.Kill(pid, 0) == nil
	}
	if exitCode, err := s.ReadExitCode(); err == nil {
		info.ExitCode = &exitCode
	}
	return info
}

func debugUsageDetails(s *session.Session, state *iruntime.State) debugUsageInfo {
	var tokens usage.TokenUsage
	if state != nil {
		tokens = usage.TokenUsage{
			InputTokens:  state.Usage.InputTokens,
			OutputTokens: state.Usage.OutputTokens,
			CacheRead:    state.Usage.CacheRead,
			CacheCreate:  state.Usage.CacheCreate,
		}
	} else {
		tokens = usage.ForSession(s)
	}
	return debugUsageInfo{
		InputTokens:  tokens.InputTokens,
		OutputTokens: tokens.OutputTokens,
		CacheRead:    tokens.CacheRead,
		CacheCreate:  tokens.CacheCreate,
		TotalTokens:  tokens.Total(),
	}
}

func debugExitReason(s *session.Session, state *iruntime.State, status string) string {
	if state != nil && state.CrashInfo != nil && state.CrashInfo.PanicMessage != "" {
		return "panic: " + state.CrashInfo.PanicMessage
	}
	if state != nil && state.LastError != "" {
		return state.LastError
	}
	if exitCode, err := s.ReadExitCode(); err == nil {
		if exitCode == 0 {
			return "exit code 0"
		}
		return fmt.Sprintf("exit code %d", exitCode)
	}
	if status == session.StatusZombie {
		return "process exited before finalizing output"
	}
	if status == session.StatusCancelled {
		return "cancelled"
	}
	return ""
}

func readDebugArtifact(paths ...string) *debugArtifact {
	for _, path := range paths {
		if path == "" {
			continue
		}
		data, err := iruntime.ReadDiagnosticTail(path)
		if err != nil {
			continue
		}
		return &debugArtifact{Path: path, Content: string(data)}
	}
	return nil
}

func debugMetadataArtifacts(s *session.Session) []debugBundleArtifact {
	artifacts := []debugBundleArtifact{
		{Name: "state.json", Path: iruntime.StatePath(s)},
		{Name: "events.jsonl", Path: iruntime.EventLogPath(s)},
		{Name: "stderr.log", Path: iruntime.StderrLogPath(s)},
		{Name: "toc-output.txt", Path: s.WorkspacePath + "/toc-output.txt"},
		{Name: "toc-output.txt.tmp", Path: s.WorkspacePath + "/toc-output.txt.tmp"},
	}

	var existing []debugBundleArtifact
	for _, artifact := range artifacts {
		if artifact.Path == "" {
			continue
		}
		if _, err := os.Stat(artifact.Path); err == nil {
			existing = append(existing, artifact)
		}
	}
	return existing
}

func writeDebugBundle(path string, report *debugReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	summary, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		_ = f.Close()
		return err
	}
	if err := writeTarEntry(tw, "summary.json", append(summary, '\n')); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		_ = f.Close()
		return err
	}

	for _, artifact := range report.MetadataFiles {
		data, err := iruntime.ReadDiagnosticTail(artifact.Path)
		if err != nil {
			continue
		}
		if err := writeTarEntry(tw, artifact.Name, data); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = f.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = f.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0600,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func printDebugReport(report *debugReport, full bool) {
	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Bold("Session:"), ui.Cyan(report.Session.ID))
	fmt.Printf("  %s %s\n", ui.Bold("Agent:"), ui.Cyan(report.Session.Agent))
	fmt.Printf("  %s %s\n", ui.Bold("Status:"), ui.Dim(report.Session.Status))
	if report.State.Model != "" {
		fmt.Printf("  %s %s\n", ui.Bold("Model:"), ui.Dim(report.State.Model))
	}
	if report.Session.StartTime.IsZero() {
		fmt.Printf("  %s %s\n", ui.Bold("Started:"), ui.Dim(report.Session.CreatedAt.Format(time.RFC3339)))
	} else {
		fmt.Printf("  %s %s\n", ui.Bold("Started:"), ui.Dim(report.Session.StartTime.Format(time.RFC3339)))
	}
	if report.Session.Duration != "" {
		fmt.Printf("  %s %s\n", ui.Bold("Duration:"), ui.Dim(report.Session.Duration))
	}
	if report.Session.ExitReason != "" {
		fmt.Printf("  %s %s\n", ui.Bold("Exit reason:"), ui.Dim(report.Session.ExitReason))
	}
	fmt.Println()

	fmt.Printf("  %s\n", ui.Bold("State"))
	if report.State.RuntimeStatus != "" {
		fmt.Printf("    runtime: %s\n", report.State.RuntimeStatus)
	}
	if report.State.LastError != "" {
		fmt.Printf("    last_error: %s\n", report.State.LastError)
	}
	if report.State.PendingTurnLabel != "" && report.State.PendingTurn != nil {
		fmt.Printf("    pending_turn: %s\n", report.State.PendingTurnLabel)
	}
	if report.State.ResumeCount > 0 || report.State.RecoveryCount > 0 || report.State.CompactionCount > 0 {
		fmt.Printf("    resumes=%d recoveries=%d compactions=%d\n", report.State.ResumeCount, report.State.RecoveryCount, report.State.CompactionCount)
	}
	if report.State.CrashInfo != nil {
		if !report.State.CrashInfo.CrashTime.IsZero() {
			fmt.Printf("    crash_time: %s\n", report.State.CrashInfo.CrashTime.Format(time.RFC3339))
		}
		if report.State.CrashInfo.LastToolCall != "" {
			fmt.Printf("    last_tool_call: %s\n", report.State.CrashInfo.LastToolCall)
		}
	}
	fmt.Println()

	fmt.Printf("  %s\n", ui.Bold("Timeline"))
	fmt.Printf("    total_events: %d\n", report.Timeline.TotalEvents)
	if len(report.Timeline.EventTypes) > 0 {
		fmt.Printf("    event_types: %s\n", formatEventTypes(report.Timeline.EventTypes))
	}
	if report.Timeline.LastToolCall != "" {
		fmt.Printf("    last_tool_call: %s\n", report.Timeline.LastToolCall)
	}
	if report.Timeline.LastErrorEvent != nil {
		fmt.Printf("    last_error_event: %s\n", strings.TrimSpace(report.Timeline.LastErrorEvent.Step.Content))
	}
	if len(report.Timeline.RecentEvents) > 0 {
		fmt.Printf("    recent_events:\n")
		for _, event := range report.Timeline.RecentEvents {
			label := event.Step.Type
			if event.Step.Tool != "" {
				label += "/" + event.Step.Tool
			}
			body := strings.TrimSpace(event.Step.Content)
			if body == "" && event.Step.StackTrace != "" {
				body = "stack trace captured"
			}
			if len(body) > 120 {
				body = body[:117] + "..."
			}
			fmt.Printf("      - %s %s\n", label, body)
		}
	}
	fmt.Println()

	fmt.Printf("  %s\n", ui.Bold("Process"))
	if report.Process.PID != nil {
		fmt.Printf("    pid: %d\n", *report.Process.PID)
	}
	fmt.Printf("    alive: %t\n", report.Process.Alive)
	fmt.Printf("    zombie: %t\n", report.Process.Zombie)
	if report.Process.ExitCode != nil {
		fmt.Printf("    exit_code: %d\n", *report.Process.ExitCode)
	}
	fmt.Println()

	fmt.Printf("  %s\n", ui.Bold("Usage"))
	fmt.Printf("    input=%d output=%d cache_read=%d cache_create=%d total=%d\n", report.Usage.InputTokens, report.Usage.OutputTokens, report.Usage.CacheRead, report.Usage.CacheCreate, report.Usage.TotalTokens)
	fmt.Printf("    cost_usd: unavailable\n")
	fmt.Println()

	printArtifact("toc-output", report.Output)
	printArtifact("stderr.log", report.Stderr)

	if full {
		printFullPayload("full_state", report.FullState)
		printFullPayload("full_events", report.FullEvents)
	}
}

func printArtifact(label string, artifact *debugArtifact) {
	fmt.Printf("  %s\n", ui.Bold(label))
	if artifact == nil {
		fmt.Printf("    (missing)\n\n")
		return
	}
	if artifact.Path != "" {
		fmt.Printf("    path: %s\n", artifact.Path)
	}
	content := strings.TrimSpace(artifact.Content)
	if content == "" {
		fmt.Printf("    (empty)\n\n")
		return
	}
	fmt.Println(indentBlock(content, "    "))
	fmt.Println()
}

func printFullPayload(label string, value interface{}) {
	fmt.Printf("  %s\n", ui.Bold(label))
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Printf("    failed to render: %v\n\n", err)
		return
	}
	fmt.Println(indentBlock(string(data), "    "))
	fmt.Println()
}

func printFailedDebugSessions(items []debugFailedSession) {
	if len(items) == 0 {
		ui.Info("No failed or zombie sessions found.")
		return
	}
	fmt.Println()
	for _, item := range items {
		line := fmt.Sprintf("  %s  %s  %s  %s", ui.Dim(item.ID[:8]), ui.Cyan(item.Agent), ui.Dim(item.Status), ui.Dim(item.CreatedAt.Format(time.RFC3339)))
		if item.Name != "" {
			line += "  " + ui.Dim(item.Name)
		}
		fmt.Println(line)
		if item.LastError != "" {
			fmt.Printf("    last_error: %s\n", item.LastError)
		}
	}
	fmt.Println()
}

func formatEventTypes(typeCounts map[string]int) string {
	var keys []string
	for key := range typeCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, typeCounts[key]))
	}
	return strings.Join(parts, ", ")
}

func indentBlock(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

func stateField[T any](state *iruntime.State, get func(*iruntime.State) T) T {
	var zero T
	if state == nil {
		return zero
	}
	return get(state)
}
