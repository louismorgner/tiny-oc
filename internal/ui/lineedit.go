package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
)

// LineEditor reads a single line of input in raw mode, supporting
// Alt+Backspace (ESC DEL) for word deletion in addition to standard
// editing keys (Backspace, Ctrl+W, Ctrl+U, Ctrl+C, Ctrl+D).
type LineEditor struct {
	In     *os.File
	Out    io.Writer
	Prompt string // prompt string reprinted on redraw
}

// errInterrupt is returned when the user presses Ctrl+C.
type errInterrupt struct{}

func (errInterrupt) Error() string { return "interrupt" }

// IsInterrupt reports whether err was caused by Ctrl+C.
func IsInterrupt(err error) bool {
	_, ok := err.(errInterrupt)
	return ok
}

// ReadLine puts the terminal into raw mode and reads one line of input.
// It returns the line (without trailing newline) or an error.
// On Ctrl+C it returns errInterrupt; on Ctrl+D at an empty line it returns io.EOF.
func (le *LineEditor) ReadLine() (string, error) {
	oldState, err := term.MakeRaw(le.In.Fd())
	if err != nil {
		return "", fmt.Errorf("lineedit: raw mode: %w", err)
	}
	defer term.Restore(le.In.Fd(), oldState)

	return readLineRaw(le.In, le.Out, le.Prompt)
}

// readLineRaw implements the core line editing loop. It reads byte-by-byte
// from r and echoes to w. The caller is responsible for putting the terminal
// into raw mode before calling this function.
func readLineRaw(r io.Reader, w io.Writer, prompt string) (string, error) {
	br := bufio.NewReader(r)
	var buf []byte

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

		case b == 0x17: // Ctrl+W — delete word backward
			buf = deleteWordBackward(buf)
			redraw()

		case b == 0x15: // Ctrl+U — kill line
			buf = buf[:0]
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
				fmt.Fprint(w, "\b \b")
			}

		case b >= 0x20: // Printable ASCII and start of UTF-8 sequences
			buf = append(buf, b)
			w.Write([]byte{b})

		default:
			// Other control characters: ignore
		}
	}
}

// discardEscapeSequence reads and discards bytes that are part of an ANSI
// escape sequence (e.g. arrow keys send ESC [ A). It consumes all bytes
// that are currently buffered and look like part of a CSI sequence.
func discardEscapeSequence(br *bufio.Reader) {
	// CSI sequences start with '[' and end with a letter.
	// Other sequences (SS2/SS3) are just ESC followed by one char.
	next, err := br.ReadByte()
	if err != nil {
		return
	}
	if next != '[' {
		// Not a CSI sequence; single-char escape — already consumed.
		return
	}
	// CSI: read until a final byte (0x40-0x7E).
	for {
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
