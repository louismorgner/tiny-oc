package tail

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/replay"
)

// Event represents a parsed step or a completion signal from a tailed session.
type Event struct {
	Step     replay.Step
	Finished bool   // true when the session completed
	ExitCode string // non-empty when Finished is true
}

// Options configures the tailer.
type Options struct {
	JSONLPath     string        // path to the .jsonl file
	WorkspacePath string        // sub-agent workspace (to check for toc-output.txt)
	PollInterval  time.Duration // default 500ms
}

// Tail streams events from a JSONL file until the session finishes or ctx is cancelled.
// It reads any existing content first (catching up), then polls for new content.
// The returned channel is closed when tailing stops.
func Tail(ctx context.Context, opts Options) (<-chan Event, error) {
	if opts.PollInterval == 0 {
		opts.PollInterval = 500 * time.Millisecond
	}

	ch := make(chan Event, 64)

	go func() {
		defer close(ch)

		var offset int64
		var partial []byte // buffer for incomplete trailing line

		ticker := time.NewTicker(opts.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check if JSONL file exists yet
				fi, err := os.Stat(opts.JSONLPath)
				if err != nil {
					// File doesn't exist yet — check if session already finished without writing
					if isSessionFinished(opts.WorkspacePath) {
						emitFinished(ch, opts.WorkspacePath)
						return
					}
					continue
				}

				// Read new data if file grew
				if fi.Size() > offset {
					newSteps, newOffset, newPartial := readNewLines(opts.JSONLPath, offset, partial)
					offset = newOffset
					partial = newPartial

					for _, step := range newSteps {
						select {
						case ch <- Event{Step: step}:
						case <-ctx.Done():
							return
						}
					}
				}

				// Check for session completion
				if isSessionFinished(opts.WorkspacePath) {
					// Final read to catch any last writes
					if fi2, err := os.Stat(opts.JSONLPath); err == nil && fi2.Size() > offset {
						newSteps, _, _ := readNewLines(opts.JSONLPath, offset, partial)
						for _, step := range newSteps {
							select {
							case ch <- Event{Step: step}:
							case <-ctx.Done():
								return
							}
						}
					}
					emitFinished(ch, opts.WorkspacePath)
					return
				}
			}
		}
	}()

	return ch, nil
}

func readNewLines(path string, offset int64, partial []byte) ([]replay.Step, int64, []byte) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, partial
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, partial
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, offset, partial
	}

	newOffset := offset + int64(len(data))

	// Prepend any partial line from last read
	if len(partial) > 0 {
		data = append(partial, data...)
		partial = nil
	}

	// Split into lines; last element may be incomplete
	lines := strings.Split(string(data), "\n")

	// If data doesn't end with newline, last element is a partial line
	if len(data) > 0 && data[len(data)-1] != '\n' {
		partial = []byte(lines[len(lines)-1])
		lines = lines[:len(lines)-1]
	}

	var steps []replay.Step
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if parsed := replay.ParseJSONLLine([]byte(line)); parsed != nil {
			steps = append(steps, parsed...)
		}
	}

	return steps, newOffset, partial
}

func isSessionFinished(workspacePath string) bool {
	_, err := os.Stat(filepath.Join(workspacePath, "toc-output.txt"))
	return err == nil
}

func emitFinished(ch chan<- Event, workspacePath string) {
	exitCode := "0"
	if data, err := os.ReadFile(filepath.Join(workspacePath, "toc-exit-code.txt")); err == nil {
		exitCode = strings.TrimSpace(string(data))
	}
	ch <- Event{Finished: true, ExitCode: exitCode}
}
