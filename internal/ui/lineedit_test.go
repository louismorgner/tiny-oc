package ui

import (
	"bytes"
	"io"
	"testing"
)

func TestReadLineRawSimpleInput(t *testing.T) {
	in := bytes.NewReader([]byte("hello\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "")
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

	line, err := readLineRaw(in, &out, "")
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

	line, err := readLineRaw(in, &out, "")
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

	line, err := readLineRaw(in, &out, "")
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

	_, err := readLineRaw(in, &out, "")
	if !IsInterrupt(err) {
		t.Errorf("expected interrupt error, got %v", err)
	}
}

func TestReadLineRawCtrlDEmpty(t *testing.T) {
	in := bytes.NewReader([]byte("\x04"))
	var out bytes.Buffer

	_, err := readLineRaw(in, &out, "")
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReadLineRawCtrlU(t *testing.T) {
	in := bytes.NewReader([]byte("hello world\x15new\r"))
	var out bytes.Buffer

	line, err := readLineRaw(in, &out, "")
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

	line, err := readLineRaw(in, &out, "")
	if err != nil {
		t.Fatal(err)
	}
	if line != "hello " {
		t.Errorf("got %q, want %q", line, "hello ")
	}
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
