package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
)

// LineEditor reads a single line of input in raw mode, supporting
// Alt+Backspace (ESC DEL) for word deletion in addition to standard
// editing keys (Backspace, Ctrl+W, Ctrl+U, Ctrl+C, Ctrl+D).
type LineEditor struct {
	In  *os.File
	Out io.Writer

	mu       sync.Mutex
	buf      []byte      // snapshot of line buffer for external readers, guarded by mu
	oldState *term.State // saved terminal state from EnterRawMode, guarded by mu
}

// errInterrupt is returned when the user presses Ctrl+C.
type errInterrupt struct{}

func (errInterrupt) Error() string { return "interrupt" }

// IsInterrupt reports whether err was caused by Ctrl+C.
func IsInterrupt(err error) bool {
	_, ok := err.(errInterrupt)
	return ok
}

// EnterRawMode puts the terminal into raw mode. The caller must call
// RestoreMode to restore the original terminal state. This allows the
// caller to ensure the terminal is restored even if the ReadLine
// goroutine is still blocked on input.
func (le *LineEditor) EnterRawMode() error {
	old, err := term.MakeRaw(le.In.Fd())
	if err != nil {
		return fmt.Errorf("lineedit: raw mode: %w", err)
	}
	le.oldState = old
	return nil
}

// RestoreMode restores the terminal to the state saved by EnterRawMode.
// It is safe to call concurrently and multiple times.
func (le *LineEditor) RestoreMode() {
	le.mu.Lock()
	defer le.mu.Unlock()
	if le.oldState != nil {
		term.Restore(le.In.Fd(), le.oldState)
		le.oldState = nil
	}
}

// ReadLine reads one line of input. The caller must have already called
// EnterRawMode. The prompt string is used when redrawing the line (e.g.
// after word deletion). It returns the line (without trailing newline)
// or an error. On Ctrl+C it returns errInterrupt; on Ctrl+D at an empty
// line it returns io.EOF.
func (le *LineEditor) ReadLine(prompt string) (string, error) {
	le.mu.Lock()
	le.buf = le.buf[:0]
	le.mu.Unlock()

	return readLineRaw(le.In, le.Out, prompt, le)
}

// Buffer returns a copy of the current in-progress input. This is safe
// to call from another goroutine while ReadLine is running.
func (le *LineEditor) Buffer() []byte {
	le.mu.Lock()
	defer le.mu.Unlock()
	cp := make([]byte, len(le.buf))
	copy(cp, le.buf)
	return cp
}

// setBuf replaces the internal buffer (called from readLineRaw).
func (le *LineEditor) setBuf(b []byte) {
	le.mu.Lock()
	le.buf = append(le.buf[:0], b...)
	le.mu.Unlock()
}

// readLineRaw implements the core line editing loop. It reads byte-by-byte
// from r and echoes to w. The caller is responsible for putting the terminal
// into raw mode before calling this function. If le is non-nil, it is used
// to synchronize the buffer for external readers.
func readLineRaw(r io.Reader, w io.Writer, prompt string, le *LineEditor) (string, error) {
	br := bufio.NewReader(r)
	var buf []byte

	syncBuf := func() {
		if le != nil {
			le.setBuf(buf)
		}
	}

	redraw := func() {
		fmt.Fprintf(w, "\r\033[2K")
		if prompt != "" {
			fmt.Fprint(w, prompt)
		}
		w.Write(buf)
	}

	for {
		b, err := br.ReadByte()
		if err != nil {
			return string(buf), err
		}

		switch {
		case b == 0x0d || b == 0x0a: // Enter
			fmt.Fprint(w, "\r\n")
			return string(buf), nil

		case b == 0x03: // Ctrl+C
			fmt.Fprint(w, "\r\n")
			return "", errInterrupt{}

		case b == 0x04: // Ctrl+D
			if len(buf) == 0 {
				fmt.Fprint(w, "\r\n")
				return "", io.EOF
			}
			// Non-empty buffer: ignore. We don't support cursor movement,
			// so delete-char-under-cursor (readline default) is not applicable.
			// Flushing the partial buffer would be surprising. Silently
			// ignoring matches the behavior of simple line editors.

		case b == 0x17: // Ctrl+W — delete word backward
			buf = deleteWordBackward(buf)
			syncBuf()
			redraw()

		case b == 0x15: // Ctrl+U — kill line
			buf = buf[:0]
			syncBuf()
			redraw()

		case b == 0x1b: // ESC — possible Alt+key sequence
			// Peek at the next byte to check for Alt+Backspace (ESC DEL).
			next, err := br.ReadByte()
			if err != nil {
				// ESC at EOF: ignore
				return string(buf), err
			}
			if next == 0x7f {
				buf = deleteWordBackward(buf)
				syncBuf()
				redraw()
			} else {
				// Some other escape sequence (e.g. arrow keys: ESC [ A).
				// Discard the rest of the sequence by draining buffered bytes.
				br.UnreadByte()
				discardEscapeSequence(br)
			}

		case b == 0x7f || b == 0x08: // Backspace (DEL) or Ctrl+H
			if len(buf) > 0 {
				_, size := utf8.DecodeLastRune(buf)
				buf = buf[:len(buf)-size]
				syncBuf()
				fmt.Fprint(w, "\b \b")
			}

		case b >= 0x20: // Printable ASCII and start of UTF-8 sequences
			if b < 0x80 {
				// Single-byte ASCII character.
				buf = append(buf, b)
				syncBuf()
				w.Write([]byte{b})
			} else {
				// Multi-byte UTF-8: unread the leading byte and use
				// ReadRune to collect the complete character.
				br.UnreadByte()
				r, size, err := br.ReadRune()
				if err != nil {
					return string(buf), err
				}
				if r == utf8.RuneError && size == 1 {
					// Invalid UTF-8 byte — skip it.
					continue
				}
				var rBuf [utf8.UTFMax]byte
				n := utf8.EncodeRune(rBuf[:], r)
				buf = append(buf, rBuf[:n]...)
				syncBuf()
				w.Write(rBuf[:n])
			}

		default:
			// Other control characters: ignore
		}
	}
}

// maxCSIParams is the maximum number of bytes to consume in a CSI sequence
// before giving up. Real CSI sequences rarely exceed a handful of bytes.
const maxCSIParams = 16

// discardEscapeSequence reads and discards bytes that are part of an ANSI
// escape sequence (e.g. arrow keys send ESC [ A). It only consumes bytes
// already buffered to avoid blocking on truncated sequences.
func discardEscapeSequence(br *bufio.Reader) {
	// CSI sequences start with '[' and end with a letter.
	// Other sequences (SS2/SS3) are just ESC followed by one char.
	if br.Buffered() == 0 {
		return
	}
	next, err := br.ReadByte()
	if err != nil {
		return
	}
	if next == 'O' {
		// SS3 sequence (e.g. ESC O H for Home, ESC O F for End).
		// Consume the final byte to avoid leaking it into input.
		if br.Buffered() > 0 {
			br.ReadByte()
		}
		return
	}
	if next != '[' {
		// Not a CSI or SS3 sequence; single-char escape — already consumed.
		return
	}
	// CSI: read until a final byte (0x40-0x7E), but only from what's
	// already buffered and with a bounded iteration count.
	for i := 0; i < maxCSIParams; i++ {
		if br.Buffered() == 0 {
			return
		}
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		if b >= 0x40 && b <= 0x7E {
			return
		}
	}
}

// deleteWordBackward removes the previous word from buf, matching standard
// terminal behavior: skip trailing whitespace, then delete until the next
// whitespace or beginning of line.
func deleteWordBackward(buf []byte) []byte {
	// Skip trailing whitespace
	for len(buf) > 0 {
		r, size := utf8.DecodeLastRune(buf)
		if !unicode.IsSpace(r) {
			break
		}
		buf = buf[:len(buf)-size]
	}
	// Delete word characters
	for len(buf) > 0 {
		r, size := utf8.DecodeLastRune(buf)
		if unicode.IsSpace(r) {
			break
		}
		buf = buf[:len(buf)-size]
	}
	return buf
}
