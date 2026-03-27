package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeEditRejectsEmptyOldString(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeEdit(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Edit", map[string]interface{}{
		"file_path":  "notes.txt",
		"old_string": "",
		"new_string": "updated",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if !strings.Contains(result.Message, "old_string must not be empty") {
		t.Fatalf("unexpected edit failure: %q", result.Message)
	}
}

func TestNativeWriteAndEditPreserveFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho old\n"), 0755); err != nil {
		t.Fatal(err)
	}

	writeResult := nativeWrite(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Write", map[string]interface{}{
		"file_path": "script.sh",
		"content":   "#!/bin/sh\necho new\n",
	}))
	if writeResult.Step.Success == nil || !*writeResult.Step.Success {
		t.Fatalf("write failed: %#v", writeResult)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode after write = %#o, want 0755", info.Mode().Perm())
	}

	editResult := nativeEdit(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Edit", map[string]interface{}{
		"file_path":  "script.sh",
		"old_string": "echo new",
		"new_string": "echo edited",
	}))
	if editResult.Step.Success == nil || !*editResult.Step.Success {
		t.Fatalf("edit failed: %#v", editResult)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode after edit = %#o, want 0755", info.Mode().Perm())
	}
}

func TestNativeGlobReturnsSessionRelativePathsAndSkipsRuntimeDirs(t *testing.T) {
	dir := t.TempDir()
	for _, subdir := range []string{
		filepath.Join(dir, "src"),
		filepath.Join(dir, ".git"),
		filepath.Join(dir, ".toc-native"),
	} {
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".toc-native", "system-prompt.md"), []byte("system\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeGlob(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Glob", map[string]interface{}{
		"pattern": "*.go",
		"path":    "src",
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("glob failed: %#v", result)
	}
	if strings.TrimSpace(result.Message) != "src/main.go" {
		t.Fatalf("glob result = %q", result.Message)
	}

	all := nativeGlob(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Glob", map[string]interface{}{
		"pattern": "**/*",
	}))
	if strings.Contains(all.Message, ".git/config") || strings.Contains(all.Message, ".toc-native/system-prompt.md") {
		t.Fatalf("glob should skip runtime dirs, got %q", all.Message)
	}
}

func TestNativeGrepIncludesHiddenFilesAndSkipsGitDirs(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("TOKEN=git-only\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeGrep(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Grep", map[string]interface{}{
		"pattern": "TOKEN",
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("grep failed: %#v", result)
	}
	if !strings.Contains(result.Message, ".env:1:TOKEN=secret") {
		t.Fatalf("grep missing hidden file match: %q", result.Message)
	}
	if strings.Contains(result.Message, ".git/config") {
		t.Fatalf("grep should skip .git internals: %q", result.Message)
	}
}

func TestNativeBashReportsExitCode(t *testing.T) {
	dir := t.TempDir()
	result := nativeBash(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Bash", map[string]interface{}{
		"command": "printf 'oops'; exit 7",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected bash failure, got %#v", result)
	}
	if result.Step.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.Step.ExitCode)
	}
	if result.Step.DurationMS <= 0 {
		t.Fatalf("expected duration, got %#v", result.Step)
	}
	if !strings.Contains(result.Message, "oops") || !strings.Contains(result.Message, "command exited with code 7") {
		t.Fatalf("unexpected bash failure message: %q", result.Message)
	}
}

func TestNativeBashReportsTimeout(t *testing.T) {
	dir := t.TempDir()
	result := nativeBash(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Bash", map[string]interface{}{
		"command":    "sleep 1",
		"timeout_ms": 10,
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected bash timeout failure, got %#v", result)
	}
	if !result.Step.TimedOut {
		t.Fatalf("expected timeout flag, got %#v", result.Step)
	}
	if result.Step.DurationMS <= 0 {
		t.Fatalf("expected duration, got %#v", result.Step)
	}
	if !strings.Contains(result.Message, "timed out") {
		t.Fatalf("unexpected timeout message: %q", result.Message)
	}
}

func TestNativeBashRejectsEmptyCommand(t *testing.T) {
	dir := t.TempDir()
	result := nativeBash(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Bash", map[string]interface{}{
		"command": "   ",
	}))

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected bash failure, got %#v", result)
	}
	if !strings.Contains(result.Message, "command is required") {
		t.Fatalf("unexpected empty command message: %q", result.Message)
	}
}

func TestNativeReadCatNFormat(t *testing.T) {
	dir := t.TempDir()
	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeRead(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Read", map[string]interface{}{
		"file_path": "test.txt",
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("read failed: %#v", result)
	}
	// Output should be in cat -n format with 6-char right-aligned line numbers
	if !strings.Contains(result.Message, "     1\tline one") {
		t.Fatalf("expected cat -n format, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "     2\tline two") {
		t.Fatalf("expected line 2 in cat -n format, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "     3\tline three") {
		t.Fatalf("expected line 3 in cat -n format, got %q", result.Message)
	}
	// Trailing newline in file must not produce a phantom empty line 4.
	if strings.Contains(result.Message, "     4\t") {
		t.Fatalf("trailing newline produced phantom empty line: %q", result.Message)
	}
}

func TestNativeReadOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Read from offset 50, limit 5
	result := nativeRead(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Read", map[string]interface{}{
		"file_path": "big.txt",
		"offset":    50,
		"limit":     5,
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("read failed: %#v", result)
	}
	if !strings.Contains(result.Message, "    50\tline 50") {
		t.Fatalf("expected line 50, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "    54\tline 54") {
		t.Fatalf("expected line 54, got %q", result.Message)
	}
	if strings.Contains(result.Message, "    55\t") {
		t.Fatalf("should not contain line 55, got %q", result.Message)
	}
	if strings.Contains(result.Message, "    49\t") {
		t.Fatalf("should not contain line 49, got %q", result.Message)
	}
	if result.Step.Lines != 5 {
		t.Fatalf("expected 5 lines, got %d", result.Step.Lines)
	}
}

func TestNativeReadDefaultLimitCapsAt2000(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 2500; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "huge.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := nativeRead(nativeToolContext{SessionDir: dir, Agent: "tester"}, toolCall(t, "Read", map[string]interface{}{
		"file_path": "huge.txt",
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("read failed: %#v", result)
	}
	// Should cap at 2000 lines (line 1 through 2000)
	if !strings.Contains(result.Message, "  2000\tline 2000") {
		t.Fatalf("expected line 2000, got tail: %q", result.Message[len(result.Message)-100:])
	}
	if strings.Contains(result.Message, "  2001\tline 2001") {
		t.Fatalf("should not contain line 2001")
	}
	if result.Step.Lines != 2000 {
		t.Fatalf("expected 2000 lines, got %d", result.Step.Lines)
	}
}

func TestFormatCatN(t *testing.T) {
	lines := []string{"alpha", "beta", "gamma", "delta"}

	got := formatCatN(lines, 2, 3)
	want := "     2\tbeta\n     3\tgamma\n"
	if got != want {
		t.Fatalf("formatCatN(2,3) = %q, want %q", got, want)
	}

	// Out of range
	got = formatCatN(lines, 10, 20)
	if got != "" {
		t.Fatalf("formatCatN(10,20) = %q, want empty", got)
	}

	// endLine beyond length
	got = formatCatN(lines, 3, 100)
	want = "     3\tgamma\n     4\tdelta\n"
	if got != want {
		t.Fatalf("formatCatN(3,100) = %q, want %q", got, want)
	}
}

func toolCall(t *testing.T, name string, args map[string]interface{}) ToolCall {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return ToolCall{
		ID: "call-1",
		Function: ToolCallFunction{
			Name:      name,
			Arguments: string(data),
		},
	}
}
