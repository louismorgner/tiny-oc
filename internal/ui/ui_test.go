package ui_test

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/tiny-oc/toc/internal/ui"
)

func TestWordDeleteFilterRemapsCtrlBackspace(t *testing.T) {
	// Ctrl+Backspace in Kitty protocol produces {Code: KeyBackspace, Mod: ModCtrl}.
	// The WordDeleteFilter should remap it to ctrl+w so that the textinput
	// recognises it as word deletion.
	orig := tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModCtrl}
	result := ui.WordDeleteFilter(nil, orig)

	remapped, ok := result.(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", result)
	}
	if remapped.String() != "ctrl+w" {
		t.Errorf("expected remapped key to be ctrl+w, got %q", remapped.String())
	}

	km := textinput.DefaultKeyMap()
	if !key.Matches(remapped, km.DeleteWordBackward) {
		t.Error("remapped key should match DeleteWordBackward")
	}
}

func TestWordDeleteFilterPassthroughRegularBackspace(t *testing.T) {
	// Regular backspace should NOT be remapped
	orig := tea.KeyPressMsg{Code: tea.KeyBackspace}
	result := ui.WordDeleteFilter(nil, orig)

	passed, ok := result.(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", result)
	}
	if passed.String() != "backspace" {
		t.Errorf("regular backspace should pass through unchanged, got %q", passed.String())
	}
}

func TestWordDeletionCtrlW(t *testing.T) {
	m := textinput.New()
	m.Focus()
	m.SetValue("hello world foo")
	m.SetCursor(15)

	msg := tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}
	m, _ = m.Update(msg)

	if got := m.Value(); got != "hello world " {
		t.Errorf("after ctrl+w: got %q, want %q", got, "hello world ")
	}
}

func TestWordDeletionAltBackspace(t *testing.T) {
	m := textinput.New()
	m.Focus()
	m.SetValue("hello world foo")
	m.SetCursor(15)

	msg := tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt}
	m, _ = m.Update(msg)

	if got := m.Value(); got != "hello world " {
		t.Errorf("after alt+backspace: got %q, want %q", got, "hello world ")
	}
}

func TestWordDeletionCtrlBackspaceViaFilter(t *testing.T) {
	// End-to-end: Ctrl+Backspace → filter remaps to ctrl+w → textinput deletes word
	m := textinput.New()
	m.Focus()
	m.SetValue("hello world foo")
	m.SetCursor(15)

	orig := tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModCtrl}
	remapped := ui.WordDeleteFilter(nil, orig)
	m, _ = m.Update(remapped)

	if got := m.Value(); got != "hello world " {
		t.Errorf("after ctrl+backspace via filter: got %q, want %q", got, "hello world ")
	}
}

func TestKeyDecodingCtrlW(t *testing.T) {
	var dec uv.EventDecoder
	n, evt := dec.Decode([]byte{0x17})
	if n != 1 {
		t.Fatalf("expected 1 byte consumed, got %d", n)
	}
	kp, ok := evt.(uv.KeyPressEvent)
	if !ok {
		t.Fatalf("expected KeyPressEvent, got %T", evt)
	}

	km := textinput.DefaultKeyMap()
	msg := tea.KeyPressMsg(kp)
	if !key.Matches(msg, km.DeleteWordBackward) {
		t.Errorf("ctrl+w byte (0x17) did not match DeleteWordBackward; key=%q", msg.String())
	}
}

func TestKeyDecodingAltBackspace(t *testing.T) {
	var dec uv.EventDecoder
	n, evt := dec.Decode([]byte{0x1b, 0x7f})
	if n != 2 {
		t.Fatalf("expected 2 bytes consumed, got %d", n)
	}
	kp, ok := evt.(uv.KeyPressEvent)
	if !ok {
		t.Fatalf("expected KeyPressEvent, got %T", evt)
	}

	km := textinput.DefaultKeyMap()
	msg := tea.KeyPressMsg(kp)
	if !key.Matches(msg, km.DeleteWordBackward) {
		t.Errorf("alt+backspace bytes (0x1b 0x7f) did not match DeleteWordBackward; key=%q", msg.String())
	}
}
