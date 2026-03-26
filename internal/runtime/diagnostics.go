package runtime

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

const diagnosticTailBytes = 128 * 1024

func StderrLogPath(sess *session.Session) string {
	if dir := sess.MetadataDirPath(); dir != "" {
		return filepath.Join(dir, "stderr.log")
	}
	return ""
}

func StderrLogPathInWorkspace(workspace, sessionID string) string {
	return filepath.Join(MetadataDir(workspace, sessionID), "stderr.log")
}

func DescribePendingTurn(turn *TurnCheckpoint) string {
	return describeTurnCheckpoint(turn)
}

func LastToolCall(state *State) string {
	if state == nil {
		return ""
	}
	if state.CrashInfo != nil && strings.TrimSpace(state.CrashInfo.LastToolCall) != "" {
		return strings.TrimSpace(state.CrashInfo.LastToolCall)
	}
	if state.PendingTurn != nil {
		for _, call := range state.PendingTurn.ToolCalls {
			if name := strings.TrimSpace(call.Function.Name); name != "" {
				return name
			}
		}
	}
	for i := len(state.Messages) - 1; i >= 0; i-- {
		msg := state.Messages[i]
		if msg.Role == "tool" && strings.TrimSpace(msg.Name) != "" {
			return strings.TrimSpace(msg.Name)
		}
		for _, call := range msg.ToolCalls {
			if name := strings.TrimSpace(call.Function.Name); name != "" {
				return name
			}
		}
	}
	return ""
}

func PreserveCrashInfo(sess *session.Session) error {
	if sess == nil || sess.RuntimeName() != runtimeinfo.NativeRuntime {
		return nil
	}

	state, err := LoadState(sess)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	status := sess.ResolvedStatus()
	panicMessage, stackTrace := findCrashArtifacts(sess)
	if status != session.StatusZombie && panicMessage == "" && stackTrace == "" {
		return nil
	}
	// Already have persisted crash details and didn't discover anything new.
	if state.CrashInfo != nil && !state.CrashInfo.CrashTime.IsZero() && (panicMessage == "" || state.CrashInfo.PanicMessage != "") && (stackTrace == "" || state.CrashInfo.StackTrace != "") {
		return nil
	}

	info := state.CrashInfo
	if info == nil {
		info = &CrashInfo{}
	}
	if info.CrashTime.IsZero() {
		info.CrashTime = time.Now().UTC()
	}
	if info.PanicMessage == "" {
		info.PanicMessage = panicMessage
	}
	if info.StackTrace == "" {
		info.StackTrace = stackTrace
	}
	if info.LastToolCall == "" {
		info.LastToolCall = LastToolCall(state)
	}

	state.CrashInfo = info
	if state.LastError == "" && info.PanicMessage != "" {
		state.LastError = info.PanicMessage
	}
	if status == session.StatusZombie && state.Status != "completed" {
		state.Status = "crashed"
	}
	return SaveState(sess, state)
}

func ExtractPanicInfo(text string) (string, string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}

	start := strings.LastIndex(text, "\npanic: ")
	prefix := "panic: "
	if start >= 0 {
		start++
	} else {
		start = strings.LastIndex(text, "panic: ")
	}
	if start < 0 {
		start = strings.LastIndex(text, "\nfatal error: ")
		prefix = "fatal error: "
		if start >= 0 {
			start++
		} else {
			start = strings.LastIndex(text, "fatal error: ")
		}
	}
	if start < 0 {
		return "", ""
	}

	crashText := strings.TrimSpace(text[start:])
	firstLine := crashText
	if idx := strings.IndexByte(crashText, '\n'); idx >= 0 {
		firstLine = crashText[:idx]
	}
	message := strings.TrimSpace(strings.TrimPrefix(firstLine, prefix))
	stack := ""
	if idx := strings.IndexByte(crashText, '\n'); idx >= 0 {
		stack = strings.TrimSpace(crashText[idx+1:])
	}
	return message, stack
}

func findCrashArtifacts(sess *session.Session) (string, string) {
	paths := []string{
		StderrLogPath(sess),
		filepath.Join(sess.WorkspacePath, "toc-output.txt.tmp"),
		filepath.Join(sess.WorkspacePath, "toc-output.txt"),
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		data, err := readFileTail(path, diagnosticTailBytes)
		if err != nil || len(data) == 0 {
			continue
		}
		if msg, stack := ExtractPanicInfo(string(data)); msg != "" || stack != "" {
			return msg, stack
		}
	}
	return "", ""
}

func ReadDiagnosticTail(path string) ([]byte, error) {
	return readFileTail(path, diagnosticTailBytes)
}

func readFileTail(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	offset := int64(0)
	if size > limit {
		offset = size - limit
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if idx := strings.IndexByte(string(data), '\n'); idx >= 0 && idx+1 < len(data) {
			data = data[idx+1:]
		}
	}
	return data, nil
}
