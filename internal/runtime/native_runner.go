package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	rdebug "runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

type NativeRunOptions struct {
	Mode      string
	Dir       string
	SessionID string
	Agent     string
	Workspace string
	Model     string
	Prompt    string
	Resume    bool
	SpawnFunc SubAgentSpawnFunc
}

func RunNativeSession(opts NativeRunOptions, stdin io.Reader, stdout io.Writer) error {
	state, err := BootstrapNativeState(opts)
	if err != nil {
		return err
	}
	sessionCfg, err := LoadSessionConfigInWorkspace(opts.Workspace, opts.SessionID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if sessionCfg != nil {
		if err := ValidateSessionConfig(sessionCfg); err != nil {
			return err
		}
	}
	manifest, err := LoadPermissionManifestInWorkspace(opts.Workspace, opts.SessionID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if state.Model == "" && sessionCfg != nil {
		state.Model = sessionCfg.Model
	}
	if state.Prompt == "" && opts.Prompt != "" {
		state.Prompt = opts.Prompt
	}
	if err := ensureSystemPrompt(state); err != nil {
		return err
	}
	if err := SaveStateInWorkspace(opts.Workspace, opts.SessionID, state); err != nil {
		return err
	}
	profile := runtimeinfo.ResolveNativeProfile(state.Model)

	client, err := newOpenRouterClientFromEnv()
	if err != nil {
		return err
	}

	toolCtx := nativeToolContext{
		SessionDir: opts.Dir,
		Workspace:  opts.Workspace,
		Agent:      opts.Agent,
		SessionID:  opts.SessionID,
		Manifest:   manifest,
		Config:     sessionCfg,
		SpawnFunc:  opts.SpawnFunc,
	}
	toolSpecs := nativeToolSet(nil)
	if sessionCfg != nil {
		toolSpecs = nativeToolSet(sessionCfg.RuntimeConfig.EnabledTools)
	}

	if strings.TrimSpace(opts.Prompt) != "" {
		err := runNativePrompt(client, state, toolSpecs, profile, opts.Prompt, toolCtx, stdout, opts.Mode == "detached")
		if err != nil {
			return err
		}
		return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, true)
	}

	if opts.Mode == "detached" {
		return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, true)
	}

	isTTY := ui.IsTTY(os.Stdout)

	// Print session banner in interactive mode
	if isTTY {
		modelName := state.Model
		if modelName == "" {
			modelName = "default"
		}
		fmt.Fprint(stdout, ui.SessionBanner(opts.Agent, opts.SessionID, modelName))
	}

	// Intercept SIGINT so Ctrl+C doesn't kill the process. Instead we
	// treat it as a graceful exit and finalize the session, allowing
	// post-session hooks (context sync, resume message) to run.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	reader := bufio.NewReader(stdin)
	for {
		if isTTY {
			fmt.Fprint(stdout, ui.UserPromptPrefix(opts.Agent))
		} else {
			fmt.Fprint(stdout, ui.PlainPromptPrefix())
		}

		// Read user input, but also watch for SIGINT.
		type readResult struct {
			line string
			err  error
		}
		readCh := make(chan readResult, 1)
		go func() {
			line, err := reader.ReadString('\n')
			readCh <- readResult{line, err}
		}()

		var line string
		var readErr error
		select {
		case <-sigCh:
			// Ctrl+C: finalize and exit gracefully
			if isTTY {
				fmt.Fprintln(stdout)
			}
			return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, false)
		case r := <-readCh:
			line = r.line
			readErr = r.err
		}

		if readErr != nil {
			if readErr == io.EOF {
				line = strings.TrimSpace(line)
				if line == "" {
					return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, false)
				}
			} else {
				return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, false)
			}
		}

		line = strings.TrimSpace(line)
		if line == "" {
			if readErr == io.EOF {
				return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, false)
			}
			continue
		}
		if line == "/exit" || line == "/quit" {
			if isTTY {
				fmt.Fprintln(stdout)
			}
			return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, false)
		}

		if err := runNativePrompt(client, state, toolSpecs, profile, line, toolCtx, stdout, false); err != nil {
			// If a prompt fails, still finalize rather than bailing out
			// without running post-session hooks.
			return err
		}
		if readErr == io.EOF {
			return finalizeNativeSession(client, state, sessionCfg, toolSpecs, profile, toolCtx, stdout, false)
		}
	}
}

func BootstrapNativeState(opts NativeRunOptions) (*State, error) {
	state, err := LoadStateInWorkspace(opts.Workspace, opts.SessionID)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if state == nil {
		state = &State{
			Runtime:    runtimeinfo.NativeRuntime,
			SessionID:  opts.SessionID,
			Agent:      opts.Agent,
			Model:      opts.Model,
			Workspace:  opts.Workspace,
			SessionDir: opts.Dir,
			Mode:       opts.Mode,
			Status:     "bootstrapping",
		}
	}

	if opts.Model != "" {
		state.Model = opts.Model
	}
	state.Agent = opts.Agent
	state.Workspace = opts.Workspace
	state.SessionDir = opts.Dir
	state.Mode = opts.Mode
	if err := recoverInterruptedTurn(state); err != nil {
		return nil, err
	}
	if opts.Resume {
		state.ResumeCount++
	}
	if strings.TrimSpace(opts.Prompt) != "" {
		state.Prompt = strings.TrimSpace(opts.Prompt)
	}

	if err := SaveStateInWorkspace(opts.Workspace, opts.SessionID, state); err != nil {
		return nil, err
	}
	return state, nil
}

func recoverInterruptedTurn(state *State) error {
	if state == nil || state.PendingTurn == nil {
		return nil
	}

	recoveredAt := time.Now().UTC()
	note := fmt.Sprintf("Recovered from interrupted native turn while %s.", describeTurnCheckpoint(state.PendingTurn))
	state.RecoveryCount++
	state.LastRecovery = note
	state.LastRecoveredAt = recoveredAt
	state.PendingTurn = nil
	if state.Status == "running" || state.Status == "bootstrapping" {
		state.Status = "interrupted"
	}

	sess := &session.Session{
		ID:          state.SessionID,
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: MetadataDir(state.Workspace, state.SessionID),
	}
	return AppendEvent(sess, Event{
		Timestamp: recoveredAt,
		Step: Step{
			Type:    "recovery",
			Content: note,
		},
	})
}

func describeTurnCheckpoint(turn *TurnCheckpoint) string {
	if turn == nil {
		return "an unknown phase"
	}

	switch turn.Phase {
	case "awaiting_model":
		if turn.Prompt != "" {
			return fmt.Sprintf("waiting for the model response to %q", truncateInline(turn.Prompt, 120))
		}
		return "waiting for the model response"
	case "executing_tools":
		if len(turn.ToolCalls) == 0 {
			return "executing tool calls"
		}
		var names []string
		for _, call := range turn.ToolCalls {
			if call.Function.Name != "" {
				names = append(names, call.Function.Name)
			}
		}
		if len(names) == 0 {
			return "executing tool calls"
		}
		return fmt.Sprintf("executing tool calls: %s", strings.Join(names, ", "))
	default:
		if turn.Phase == "" {
			return "an unknown phase"
		}
		return turn.Phase
	}
}

func ensureSystemPrompt(state *State) error {
	if len(state.Messages) > 0 && state.Messages[0].Role == "system" {
		return nil
	}
	promptPath := filepath.Join(state.SessionDir, ".toc-native", "system-prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil
	}
	state.Messages = append([]Message{{Role: "system", Content: content}}, state.Messages...)
	return nil
}

func runNativePrompt(client *openRouterClient, state *State, toolSpecs []NativeToolSpec, profile runtimeinfo.NativeModelProfile, prompt string, toolCtx nativeToolContext, stdout io.Writer, detached bool) error {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}

	sess := &session.Session{
		ID:          state.SessionID,
		Runtime:     runtimeinfo.NativeRuntime,
		MetadataDir: MetadataDir(state.Workspace, state.SessionID),
	}

	state.Status = "running"
	state.LastError = ""
	state.PendingTurn = nil
	state.Messages = append(state.Messages, Message{Role: "user", Content: prompt})
	if err := SaveStateInWorkspace(state.Workspace, state.SessionID, state); err != nil {
		return err
	}

	err := runNativeLoop(client, state, toolSpecs, profile, toolCtx, sess, stdout, detached)
	if err != nil {
		state.Status = "failed"
		state.LastError = err.Error()
		state.PendingTurn = nil
		if saveErr := SaveStateInWorkspace(state.Workspace, state.SessionID, state); saveErr != nil {
			err = fmt.Errorf("%w (additionally, failed to persist state: %v)", err, saveErr)
		}
		if eventErr := AppendEvent(sess, Event{
			Timestamp: time.Now().UTC(),
			Step: Step{
				Type:    "error",
				Content: err.Error(),
			},
		}); eventErr != nil {
			err = fmt.Errorf("%w (additionally, failed to append error event: %v)", err, eventErr)
		}
		return err
	}

	if detached {
		state.Status = "completed"
	} else {
		state.Status = "ready"
	}
	state.PendingTurn = nil
	return SaveStateInWorkspace(state.Workspace, state.SessionID, state)
}

const defaultMaxIterations = 24

func runNativeLoop(client *openRouterClient, state *State, toolSpecs []NativeToolSpec, profile runtimeinfo.NativeModelProfile, toolCtx nativeToolContext, sess *session.Session, stdout io.Writer, detached bool) error {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		panicMessage := fmt.Sprint(recovered)
		stackTrace := strings.TrimSpace(string(rdebug.Stack()))
		crashTime := time.Now().UTC()
		lastToolCall := LastToolCall(state)

		state.Status = "crashed"
		state.LastError = panicMessage
		state.PendingTurn = nil
		state.CrashInfo = &CrashInfo{
			PanicMessage: panicMessage,
			StackTrace:   stackTrace,
			LastToolCall: lastToolCall,
			CrashTime:    crashTime,
		}
		_ = SaveStateInWorkspace(state.Workspace, state.SessionID, state)
		_ = AppendEvent(sess, Event{
			Timestamp: crashTime,
			Step: Step{
				Type:       "crash",
				Content:    panicMessage,
				Tool:       lastToolCall,
				StackTrace: stackTrace,
			},
		})

		panic(recovered)
	}()

	maxIterations := defaultMaxIterations
	if toolCtx.Config != nil && toolCtx.Config.RuntimeConfig.MaxIterations > 0 {
		maxIterations = toolCtx.Config.RuntimeConfig.MaxIterations
	}
	for i := 0; i < maxIterations; i++ {
		compacted, err := maybeCompactState(state, sess, toolCtx.Config)
		if err != nil {
			return err
		}
		if compacted {
			if err := SaveStateInWorkspace(state.Workspace, state.SessionID, state); err != nil {
				return err
			}
		}

		var tools []toolDefinition
		if profile.SupportsTools {
			tools = nativeToolDefinitions(toolSpecs)
		}
		state.PendingTurn = &TurnCheckpoint{
			Phase:     "awaiting_model",
			Prompt:    latestUserPrompt(state.Messages),
			StartedAt: time.Now().UTC(),
		}
		if err := SaveStateInWorkspace(state.Workspace, state.SessionID, state); err != nil {
			return err
		}

		streamEmitter := newTextStreamEmitter(sess, stdout)
		req := chatRequest{
			Model:        state.Model,
			Messages:     state.Messages,
			Tools:        tools,
			Provider:     &providerPreference{RequireParameters: true},
			CacheControl: &cacheControl{Type: "ephemeral"},
		}
		var resp *chatResponse
		if profile.SupportsStreaming {
			req.Stream = true
			resp, err = client.ChatStream(context.Background(), req, streamEmitter.WriteChunk)
			if flushErr := streamEmitter.Finish(); err == nil && flushErr != nil {
				err = flushErr
			}
		} else {
			resp, err = client.Chat(context.Background(), req)
		}
		if err != nil {
			return err
		}
		accumulateUsage(state, resp)

		msg := resp.Choices[0].Message
		state.Messages = append(state.Messages, msg)
		if len(msg.ToolCalls) > 0 {
			state.PendingTurn = &TurnCheckpoint{
				Phase:     "executing_tools",
				ToolCalls: append([]ToolCall(nil), msg.ToolCalls...),
				StartedAt: time.Now().UTC(),
			}
		} else {
			state.PendingTurn = nil
		}
		if err := SaveStateInWorkspace(state.Workspace, state.SessionID, state); err != nil {
			return err
		}

		text := extractMessageText(msg)
		if text != "" && !profile.SupportsStreaming {
			if err := AppendEvent(sess, Event{
				Timestamp: time.Now().UTC(),
				Step: Step{
					Type:    "text",
					Content: text,
				},
			}); err != nil {
				return err
			}
			if ui.IsTTY(os.Stdout) {
				fmt.Fprint(stdout, "\n")
				fmt.Fprint(stdout, ui.AssistantResponse(text))
			} else {
				if _, err := fmt.Fprintln(stdout, text); err != nil {
					return err
				}
			}
		}

		if len(msg.ToolCalls) == 0 {
			return nil
		}

		for _, call := range msg.ToolCalls {
			result := executeNativeTool(toolSpecs, toolCtx, call)

			if !detached {
				var parsedArgs map[string]interface{}
				_ = json.Unmarshal([]byte(call.Function.Arguments), &parsedArgs)
				keyParam := ui.ToolCallKeyParam(call.Function.Name, parsedArgs)
				var viz string
				if ui.IsTTY(os.Stdout) {
					viz = ui.FormatToolCallRich(call.Function.Name, keyParam, result.Message, 5)
				} else {
					viz = ui.FormatToolCall(call.Function.Name, keyParam, result.Message, 5)
				}
				fmt.Fprint(stdout, viz)
			}

			if err := AppendEvent(sess, Event{
				Timestamp: time.Now().UTC(),
				Step:      result.Step,
			}); err != nil {
				return err
			}
			state.Messages = append(state.Messages, Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    result.Message,
			})
			if err := SaveStateInWorkspace(state.Workspace, state.SessionID, state); err != nil {
				return err
			}
			if state.PendingTurn != nil && state.PendingTurn.Phase == "executing_tools" {
				state.PendingTurn.ToolCalls = remainingToolCalls(state.PendingTurn.ToolCalls, call.ID)
				if len(state.PendingTurn.ToolCalls) == 0 {
					state.PendingTurn = nil
				}
				if err := SaveStateInWorkspace(state.Workspace, state.SessionID, state); err != nil {
					return err
				}
			}
		}
	}

	return fmt.Errorf("native runtime exceeded max tool iterations")
}

func finalizeNativeSession(client *openRouterClient, state *State, sessionCfg *SessionConfig, toolSpecs []NativeToolSpec, profile runtimeinfo.NativeModelProfile, toolCtx nativeToolContext, stdout io.Writer, detached bool) error {
	if sessionCfg != nil && strings.TrimSpace(sessionCfg.OnEnd) != "" {
		if err := runNativePrompt(client, state, toolSpecs, profile, sessionCfg.OnEnd, toolCtx, stdout, detached); err != nil {
			return err
		}
	}

	if detached && sessionCfg != nil && len(sessionCfg.Context) > 0 {
		agentDir := filepath.Join(state.Workspace, ".toc", "agents", sessionCfg.Agent)
		if _, err := (nativeProvider{}).PostSessionSync(state.SessionDir, agentDir, sessionCfg.Context); err != nil {
			return err
		}
	}

	state.Status = "completed"
	return SaveStateInWorkspace(state.Workspace, state.SessionID, state)
}

func accumulateUsage(state *State, resp *chatResponse) {
	if state == nil || resp == nil {
		return
	}
	state.Usage.InputTokens += resp.Usage.PromptTokens
	state.Usage.OutputTokens += resp.Usage.CompletionTokens
	if d := resp.Usage.PromptTokensDetails; d != nil {
		state.Usage.CacheRead += d.CachedTokens
		state.Usage.CacheCreate += d.CacheWriteTokens
	}
}

type textStreamEmitter struct {
	sess             *session.Session
	stdout           io.Writer
	pending          string
	wroteOutput      bool
	lastChunkNewline bool
	// TTY buffered mode: collect all text and render as markdown at the end
	ttyMode    bool
	fullBuffer strings.Builder
	spinnerIdx int
	spinnerOn  bool
}

func newTextStreamEmitter(sess *session.Session, stdout io.Writer) *textStreamEmitter {
	isTTY := ui.IsTTY(os.Stdout)
	return &textStreamEmitter{
		sess:             sess,
		stdout:           stdout,
		lastChunkNewline: true,
		ttyMode:          isTTY,
	}
}

func (e *textStreamEmitter) WriteChunk(chunk string) error {
	if e == nil || chunk == "" {
		return nil
	}

	if e.ttyMode {
		// In TTY mode: buffer everything, show spinner
		e.fullBuffer.WriteString(chunk)
		e.wroteOutput = true

		// Show/update spinner
		if !e.spinnerOn {
			e.spinnerOn = true
			fmt.Fprint(e.stdout, ui.ThinkingIndicator(e.spinnerIdx))
		} else {
			// Update spinner frame periodically (every ~20 chunks)
			e.spinnerIdx++
			if e.spinnerIdx%20 == 0 {
				// Clear spinner line and redraw
				fmt.Fprint(e.stdout, "\r"+ui.ThinkingIndicator(e.spinnerIdx))
			}
		}

		// Still emit events for the event log
		e.pending += chunk
		for {
			idx := strings.IndexByte(e.pending, '\n')
			if idx < 0 {
				break
			}
			segment := e.pending[:idx]
			e.pending = e.pending[idx+1:]
			if err := e.emitSegment(segment); err != nil {
				return err
			}
		}
		return nil
	}

	// Non-TTY: stream directly as before
	if _, err := io.WriteString(e.stdout, chunk); err != nil {
		return err
	}
	e.wroteOutput = true
	e.lastChunkNewline = strings.HasSuffix(chunk, "\n")
	e.pending += chunk

	for {
		idx := strings.IndexByte(e.pending, '\n')
		if idx < 0 {
			break
		}
		segment := e.pending[:idx]
		e.pending = e.pending[idx+1:]
		if err := e.emitSegment(segment); err != nil {
			return err
		}
	}

	const maxBufferedSegment = 120
	for len(e.pending) >= maxBufferedSegment {
		cut := maxBufferedSegment
		if space := strings.LastIndex(e.pending[:maxBufferedSegment], " "); space > 40 {
			cut = space
		}
		segment := e.pending[:cut]
		e.pending = e.pending[cut:]
		if err := e.emitSegment(segment); err != nil {
			return err
		}
	}

	return nil
}

func (e *textStreamEmitter) Finish() error {
	if e == nil {
		return nil
	}

	// Emit any remaining buffered segments for the event log
	if err := e.emitSegment(e.pending); err != nil {
		return err
	}
	e.pending = ""

	if e.ttyMode {
		// Clear spinner line
		if e.spinnerOn {
			fmt.Fprint(e.stdout, "\r\033[K") // clear line
		}

		// Render the full response as markdown
		if e.wroteOutput {
			text := strings.TrimSpace(e.fullBuffer.String())
			if text != "" {
				fmt.Fprint(e.stdout, "\n")
				fmt.Fprint(e.stdout, ui.AssistantResponse(text))
			}
		}
		return nil
	}

	// Non-TTY: ensure trailing newline
	if e.wroteOutput && !e.lastChunkNewline {
		if _, err := io.WriteString(e.stdout, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func (e *textStreamEmitter) emitSegment(segment string) error {
	if e == nil {
		return nil
	}
	segment = strings.TrimSuffix(segment, "\r")
	if strings.TrimSpace(segment) == "" {
		return nil
	}
	if e.sess == nil {
		return nil
	}
	return AppendEvent(e.sess, Event{
		Timestamp: time.Now().UTC(),
		Step: Step{
			Type:    "text",
			Content: segment,
		},
	})
}

func latestUserPrompt(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}

func remainingToolCalls(calls []ToolCall, completedID string) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	remaining := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		if call.ID == completedID {
			continue
		}
		remaining = append(remaining, call)
	}
	return remaining
}
