package ui

import (
	"bytes"
	"io"
	"testing"
)

func TestReadLineRawSimpleInput(t *testing.T) {
	in := bytes.NewReader([]byte("hello\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "hello" {
		t.Errorf("got %q, want %q", line, "hello")
	}
}

func TestReadLineRawBackspace(t *testing.T) {
	in := bytes.NewReader([]byte("helloo\x7f\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "hello" {
		t.Errorf("got %q, want %q", line, "hello")
	}
}

func TestReadLineRawAltBackspaceDeletesWord(t *testing.T) {
	in := bytes.NewReader([]byte("hello world\x1b\x7f\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "hello " {
		t.Errorf("got %q, want %q", line, "hello ")
	}
}

func TestReadLineRawCtrlWDeletesWord(t *testing.T) {
	in := bytes.NewReader([]byte("hello world\x17\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "hello " {
		t.Errorf("got %q, want %q", line, "hello ")
	}
}

func TestReadLineRawCtrlC(t *testing.T) {
	in := bytes.NewReader([]byte("hello\x03"))
	var out bytes.Buffer

	_, err := readLineRaw(in, &out, "", nil)
	if !IsInterrupt(err) {
		t.Errorf("expected interrupt error, got %v", err)
	}
}

func TestReadLineRawCtrlDEmpty(t *testing.T) {
	in := bytes.NewReader([]byte("\x04"))
	var out bytes.Buffer

	_, err := readLineRaw(in, &out, "", nil)
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReadLineRawCtrlU(t *testing.T) {
	in := bytes.NewReader([]byte("hello world\x15new\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "new" {
		t.Errorf("got %q, want %q", line, "new")
	}
}

func TestReadLineRawMultipleAltBackspace(t *testing.T) {
	in := bytes.NewReader([]byte("hello world foo\x1b\x7f\x1b\x7f\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "hello " {
		t.Errorf("got %q, want %q", line, "hello ")
	}
}

func TestReadLineRawTruncatedCSIDoesNotBlock(t *testing.T) {
	// ESC [ with no final byte followed by normal input — the truncated
	// CSI must not block the reader.
	in := bytes.NewReader([]byte("ab\x1b[cd\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// After ESC [ the 'c' (0x63) is in the final-byte range and terminates
	// the sequence; 'd' is normal input.
	if line != "abd" {
		t.Errorf("got %q, want %q", line, "abd")
	}
}

func TestReadLineRawUTF8RoundTrip(t *testing.T) {
	// Multi-byte UTF-8 characters must round-trip correctly through the
	// line editor: buffer content should match input, and output should
	// contain the complete characters (not individual bytes).
	input := "café naïve 日本語"
	in := bytes.NewReader(append([]byte(input), '\r'))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != input {
		t.Errorf("buffer: got %q, want %q", line, input)
	}
	// The output should contain the echoed characters (may also contain
	// \r\n from Enter). Verify the input string appears in the output.
	if !bytes.Contains(out.Bytes(), []byte(input)) {
		t.Errorf("output %q does not contain %q", out.String(), input)
	}
}

func TestReadLineRawUTF8Backspace(t *testing.T) {
	// Backspace over a multi-byte character should remove the whole rune.
	// "café" + backspace should yield "caf".
	in := bytes.NewReader([]byte("café\x7f\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "caf" {
		t.Errorf("got %q, want %q", line, "caf")
	}
}

func TestReadLineRawSS3Sequence(t *testing.T) {
	// SS3 sequences like ESC O H (Home) should be fully consumed without
	// leaking the final byte into the input buffer.
	in := bytes.NewReader([]byte("ab\x1bOHcd\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "abcd" {
		t.Errorf("got %q, want %q", line, "abcd")
	}
}

func TestBufferConcurrent(t *testing.T) {
	// Exercise concurrent Buffer() access with ReadLine to verify mutex
	// safety. Run with -race to detect data races.
	r, w := io.Pipe()
	var out bytes.Buffer
	le := &LineEditor{In: nil, Out: &out}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// readLineRaw accepts any io.Reader, so we can use the pipe
		// reader directly without a real *os.File.
		readLineRaw(r, &out, "", le)
	}()

	// Write bytes slowly while concurrently reading Buffer().
	input := []byte("hello world\r")
	for _, b := range input {
		w.Write([]byte{b})
		// Concurrent read — the main thing is that -race doesn't fire.
		le.Buffer()
	}
	<-done
}

func TestDeleteWordBackward(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello "},
		{"hello world  ", "hello "},
		{"hello", ""},
		{"", ""},
		{"  ", ""},
		{"hello world foo", "hello world "},
	}
	for _, tt := range tests {
		got := string(deleteWordBackward([]byte(tt.input)))
		if got != tt.want {
			t.Errorf("deleteWordBackward(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
