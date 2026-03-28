// Mock claude binary for e2e testing.
// Accepts the same flags as real claude, writes configurable output, and exits
// with a configurable exit code. Writes a minimal JSONL conversation log so
// replay/watch work against the session directory.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	// Accept the same flags the real claude binary uses.
	printFlag := flag.Bool("print", false, "print mode (non-interactive)")
	p := flag.Bool("p", false, "print mode (short)")
	model := flag.String("model", "", "model name")
	resume := flag.String("resume", "", "resume session ID")
	sessionID := flag.String("session-id", "", "session ID")
	continueFlag := flag.Bool("continue", false, "continue previous session")
	dangerouslySkipPerms := flag.Bool("dangerously-skip-permissions", false, "skip permission prompts")
	flag.Parse()

	// Suppress unused warnings via side-effects.
	_ = model
	_ = resume
	_ = continueFlag
	_ = dangerouslySkipPerms

	// Configurable behavior via env vars.
	output := os.Getenv("MOCK_CLAUDE_OUTPUT")
	if output == "" {
		output = "mock claude output"
	}
	exitCodeStr := os.Getenv("MOCK_CLAUDE_EXIT_CODE")
	exitCode := 0
	if exitCodeStr != "" {
		fmt.Sscanf(exitCodeStr, "%d", &exitCode)
	}

	// If running in print mode (-p or --print), write output to stdout.
	// Otherwise, write to the working directory as toc-output.txt for sub-agents.
	isPrint := *printFlag || *p
	if isPrint {
		fmt.Print(output)
	} else {
		// For interactive mode, just print to stdout.
		fmt.Print(output)
	}

	if os.Getenv("MOCK_CLAUDE_API_PING") != "" {
		if err := pingAnthropicBaseURL(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	// Write a context file if requested (for context sync testing).
	if syncFile := os.Getenv("MOCK_CLAUDE_WRITE_FILE"); syncFile != "" {
		syncContent := os.Getenv("MOCK_CLAUDE_WRITE_CONTENT")
		if syncContent == "" {
			syncContent = "synced content"
		}
		dir := filepath.Dir(syncFile)
		os.MkdirAll(dir, 0755)
		os.WriteFile(syncFile, []byte(syncContent), 0644)
	}

	// Write minimal JSONL conversation log so replay/watch work.
	sid := *sessionID
	if sid == "" && *resume != "" {
		sid = *resume
	}
	if sid == "" {
		sid = "mock-session"
	}
	writeConversationLog(sid)

	os.Exit(exitCode)
}

// writeConversationLog writes a minimal JSONL file that satisfies the replay parser.
func writeConversationLog(sessionID string) {
	// Claude Code stores conversation logs in ~/.claude/projects/<hash>/sessions/
	// For testing, write to a local .claude directory in the working dir.
	logDir := filepath.Join(".claude")
	os.MkdirAll(logDir, 0755)

	logPath := filepath.Join(logDir, sessionID+".jsonl")

	entry := map[string]interface{}{
		"type":       "assistant",
		"message":    os.Getenv("MOCK_CLAUDE_OUTPUT"),
		"session_id": sessionID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(entry)
	data = append(data, '\n')

	os.WriteFile(logPath, data, 0644)
}

func pingAnthropicBaseURL() error {
	baseURL := strings.TrimRight(os.Getenv("ANTHROPIC_BASE_URL"), "/")
	if baseURL == "" {
		return fmt.Errorf("ANTHROPIC_BASE_URL is not set")
	}
	path := os.Getenv("MOCK_CLAUDE_HTTP_PATH")
	if path == "" {
		path = "/v1/messages"
	}
	body := map[string]interface{}{
		"model": "claude-sonnet-4",
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 64,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "mock-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("mock claude upstream returned %s", resp.Status)
	}
	return nil
}
