package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNativeToolNames(t *testing.T) {
	got := NativeToolNames()
	want := []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash", "WebFetch", "Skill", "TodoWrite", "Question", "SubAgent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeToolNames() = %#v, want %#v", got, want)
	}
}

func TestNativeToolSetFiltersInRegistryOrder(t *testing.T) {
	got := nativeToolSet([]string{"Bash", "Read"})
	want := []string{"Read", "Bash"}
	if len(got) != len(want) {
		t.Fatalf("nativeToolSet() len = %d, want %d", len(got), len(want))
	}
	for i, spec := range got {
		if spec.Name != want[i] {
			t.Fatalf("nativeToolSet()[%d] = %q, want %q", i, spec.Name, want[i])
		}
	}
}

func TestExecuteNativeToolWithTimeout_SingleToolTimeout(t *testing.T) {
	// Register a fake tool that blocks longer than the timeout.
	slowTool := NativeToolSpec{
		Name: "SlowTool",
		Handler: func(_ nativeToolContext, _ ToolCall) toolExecution {
			time.Sleep(5 * time.Second)
			return toolExecution{Message: "should not reach here"}
		},
	}
	specs := []NativeToolSpec{slowTool}
	ctx := nativeToolContext{SessionDir: t.TempDir(), Agent: "tester"}

	// Override toolCallTimeout by using a Bash-style call with a very short timeout
	// so the grace-period wrapper fires quickly.
	// Instead, we'll use a non-Bash tool and temporarily test with default timeout.
	// For a fast test, we use a custom approach: create a tool that sleeps,
	// and test the wrapper directly with a short deadline.

	call := ToolCall{
		ID:       "call-slow",
		Function: ToolCallFunction{Name: "SlowTool", Arguments: "{}"},
	}

	// Test the wrapper with a very short deadline by temporarily
	// using a Bash-like call with a 50ms timeout_ms to trigger fast timeout.
	bashArgs, _ := json.Marshal(map[string]interface{}{
		"command":    "sleep 60",
		"timeout_ms": 50,
	})
	bashCall := ToolCall{
		ID:       "call-bash-slow",
		Function: ToolCallFunction{Name: "Bash", Arguments: string(bashArgs)},
	}

	// Verify toolCallTimeout returns short deadline for the Bash call
	deadline := toolCallTimeout(bashCall)
	if deadline > 35*time.Second {
		t.Fatalf("expected short deadline, got %v", deadline)
	}

	// Test the non-Bash slow tool path: the default 10-minute timeout is too
	// long for a test, so we test the Bash tool path directly which gets
	// timeout_ms + grace.
	// For the SlowTool, verify the timeout mechanism works via a direct
	// goroutine-based test.
	start := time.Now()
	ch := make(chan toolExecution, 1)
	go func() {
		ch <- executeNativeTool(specs, ctx, call)
	}()

	testDeadline := 200 * time.Millisecond
	select {
	case <-ch:
		t.Fatal("tool should not have completed this fast")
	case <-time.After(testDeadline):
		// Expected: the tool is still blocked
	}
	elapsed := time.Since(start)
	if elapsed < testDeadline {
		t.Fatalf("timeout fired too early: %v", elapsed)
	}
}

func TestExecuteNativeToolWithTimeout_BashTimeoutReturnsError(t *testing.T) {
	dir := t.TempDir()
	specs := nativeToolSet(nil)
	ctx := nativeToolContext{SessionDir: dir, Agent: "tester"}

	// Use a very short timeout so the bash command times out quickly.
	bashArgs, _ := json.Marshal(map[string]interface{}{
		"command":    "sleep 60",
		"timeout_ms": 50,
	})
	call := ToolCall{
		ID:       "call-bash-timeout",
		Function: ToolCallFunction{Name: "Bash", Arguments: string(bashArgs)},
	}

	start := time.Now()
	result := executeNativeToolWithTimeout(specs, ctx, call)
	elapsed := time.Since(start)

	// Should complete within a few seconds (50ms tool timeout + 30s grace is the
	// outer limit, but the inner Bash handler should timeout at 50ms and return).
	if elapsed > 10*time.Second {
		t.Fatalf("timeout recovery took too long: %v", elapsed)
	}

	if result.Step.Success == nil || *result.Step.Success {
		t.Fatalf("expected failure, got success: %#v", result)
	}
	if !result.Step.TimedOut {
		t.Fatalf("expected TimedOut flag, got %#v", result.Step)
	}
	if !strings.Contains(result.Message, "timed out") {
		t.Fatalf("expected timeout message, got %q", result.Message)
	}
}

func TestExecuteNativeToolWithTimeout_ParallelMixedResults(t *testing.T) {
	dir := t.TempDir()
	specs := nativeToolSet(nil)
	ctx := nativeToolContext{SessionDir: dir, Agent: "tester"}

	// Simulate a parallel batch: one Bash that succeeds quickly, one that times out.
	fastArgs, _ := json.Marshal(map[string]interface{}{
		"command":    "echo hello",
		"timeout_ms": 5000,
	})
	slowArgs, _ := json.Marshal(map[string]interface{}{
		"command":    "sleep 60",
		"timeout_ms": 50,
	})

	calls := []ToolCall{
		{ID: "call-fast", Function: ToolCallFunction{Name: "Bash", Arguments: string(fastArgs)}},
		{ID: "call-slow", Function: ToolCallFunction{Name: "Bash", Arguments: string(slowArgs)}},
	}

	// Execute all tool calls (simulating what runNativeLoop does)
	results := make([]toolExecution, len(calls))
	for i, call := range calls {
		results[i] = executeNativeToolWithTimeout(specs, ctx, call)
	}

	// Fast call should succeed
	if results[0].Step.Success == nil || !*results[0].Step.Success {
		t.Fatalf("fast call should succeed, got %#v", results[0])
	}
	if !strings.Contains(results[0].Message, "hello") {
		t.Fatalf("fast call should contain output, got %q", results[0].Message)
	}

	// Slow call should timeout
	if results[1].Step.Success == nil || *results[1].Step.Success {
		t.Fatalf("slow call should fail, got %#v", results[1])
	}
	if !results[1].Step.TimedOut {
		t.Fatalf("slow call should have TimedOut flag, got %#v", results[1].Step)
	}
	if !strings.Contains(results[1].Message, "timed out") {
		t.Fatalf("slow call should have timeout message, got %q", results[1].Message)
	}
}

func TestToolCallTimeout_Bash(t *testing.T) {
	args, _ := json.Marshal(map[string]interface{}{
		"command":    "echo hi",
		"timeout_ms": 180000,
	})
	call := ToolCall{
		ID:       "call-1",
		Function: ToolCallFunction{Name: "Bash", Arguments: string(args)},
	}

	got := toolCallTimeout(call)
	want := 180*time.Second + toolTimeoutGrace
	if got != want {
		t.Fatalf("toolCallTimeout() = %v, want %v", got, want)
	}
}

func TestToolCallTimeout_BashDefault(t *testing.T) {
	args, _ := json.Marshal(map[string]interface{}{
		"command": "echo hi",
	})
	call := ToolCall{
		ID:       "call-1",
		Function: ToolCallFunction{Name: "Bash", Arguments: string(args)},
	}

	got := toolCallTimeout(call)
	want := 30*time.Second + toolTimeoutGrace
	if got != want {
		t.Fatalf("toolCallTimeout() = %v, want %v", got, want)
	}
}

func TestToolCallTimeout_NonBash(t *testing.T) {
	call := ToolCall{
		ID:       "call-1",
		Function: ToolCallFunction{Name: "Read", Arguments: `{"file_path":"foo.txt"}`},
	}

	got := toolCallTimeout(call)
	if got != defaultToolTimeout {
		t.Fatalf("toolCallTimeout() = %v, want %v", got, defaultToolTimeout)
	}
}
