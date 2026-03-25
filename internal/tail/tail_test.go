package tail

import (
	"encoding/json"
	"os"
	"testing"
)

func TestReadNewLines_CompleteLines(t *testing.T) {
	path := writeJSONLFile(t, []map[string]interface{}{
		assistantMsg("Read", map[string]string{"file_path": "a.go"}),
		assistantMsg("Bash", map[string]string{"command": "go test"}),
	})

	steps, offset, partial := readNewLines(path, 0, nil)
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	if steps[0].Tool != "Read" || steps[0].Path != "a.go" {
		t.Errorf("step 0 = %+v, want Read a.go", steps[0])
	}
	if steps[1].Tool != "Bash" || steps[1].Command != "go test" {
		t.Errorf("step 1 = %+v, want Bash 'go test'", steps[1])
	}
	if offset == 0 {
		t.Error("offset should advance past data")
	}
	if len(partial) != 0 {
		t.Errorf("partial = %q, want empty", partial)
	}
}

func TestReadNewLines_PartialLine(t *testing.T) {
	path := tempJSONLPath(t)

	// Write a complete line followed by a partial line (no trailing newline)
	line1 := marshalJSON(assistantMsg("Read", map[string]string{"file_path": "a.go"}))
	partial2 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit"`

	os.WriteFile(path, []byte(line1+"\n"+partial2), 0644)

	steps, offset, partial := readNewLines(path, 0, nil)
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1 (partial should be buffered)", len(steps))
	}
	if steps[0].Tool != "Read" {
		t.Errorf("step 0 = %+v, want Read", steps[0])
	}
	if string(partial) != partial2 {
		t.Errorf("partial = %q, want %q", partial, partial2)
	}

	// Now complete the partial line in a second read
	rest := `,"input":{"file_path":"b.go","old_string":"old","new_string":"new"}}]}}` + "\n"
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(rest)
	f.Close()

	steps, _, partial = readNewLines(path, offset, partial)
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1 (completed partial)", len(steps))
	}
	if steps[0].Tool != "Edit" || steps[0].Path != "b.go" {
		t.Errorf("step 0 = %+v, want Edit b.go", steps[0])
	}
	if len(partial) != 0 {
		t.Errorf("partial = %q, want empty after completion", partial)
	}
}

func TestReadNewLines_EmptyRead(t *testing.T) {
	path := tempJSONLPath(t)
	os.WriteFile(path, []byte(""), 0644)

	steps, offset, partial := readNewLines(path, 0, nil)
	if len(steps) != 0 {
		t.Errorf("got %d steps, want 0", len(steps))
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0", offset)
	}
	if len(partial) != 0 {
		t.Errorf("partial = %q, want empty", partial)
	}
}

func TestReadNewLines_PrependPartialBuffer(t *testing.T) {
	path := tempJSONLPath(t)

	// Previous read left a partial buffer; new data completes it
	full := marshalJSON(assistantMsg("Bash", map[string]string{"command": "ls"}))
	mid := len(full) / 2
	partialBuf := []byte(full[:mid])
	remainder := full[mid:] + "\n"

	os.WriteFile(path, []byte(remainder), 0644)

	steps, _, partial := readNewLines(path, 0, partialBuf)
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Tool != "Bash" || steps[0].Command != "ls" {
		t.Errorf("step 0 = %+v, want Bash 'ls'", steps[0])
	}
	if len(partial) != 0 {
		t.Errorf("partial = %q, want empty", partial)
	}
}

func TestReadNewLines_SkipsNonAssistant(t *testing.T) {
	path := tempJSONLPath(t)

	userLine := marshalJSON(map[string]interface{}{
		"type":    "user",
		"message": map[string]interface{}{"role": "user", "content": "hello"},
	})
	assistLine := marshalJSON(assistantMsg("Read", map[string]string{"file_path": "x.go"}))

	os.WriteFile(path, []byte(userLine+"\n"+assistLine+"\n"), 0644)

	steps, _, _ := readNewLines(path, 0, nil)
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1 (user message should be skipped)", len(steps))
	}
	if steps[0].Tool != "Read" {
		t.Errorf("step 0 = %+v, want Read", steps[0])
	}
}

// --- helpers ---

func tempJSONLPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/test.jsonl"
}

func assistantMsg(tool string, input map[string]string) map[string]interface{} {
	return map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "tool_use", "name": tool, "input": input},
			},
		},
	}
}

func marshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func writeJSONLFile(t *testing.T, entries []map[string]interface{}) string {
	t.Helper()
	path := tempJSONLPath(t)
	var content string
	for _, e := range entries {
		content += marshalJSON(e) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
