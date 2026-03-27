package session

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// startAndReap starts a command and reaps it in the background so it doesn't
// become a zombie that confuses isProcessAlive.
func startAndReap(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	return pid
}

func TestTerminateProcess_AlreadyDead(t *testing.T) {
	// Start a process and let it exit immediately.
	cmd := exec.Command("true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()

	result, err := TerminateProcess(pid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AlreadyDead {
		t.Error("expected AlreadyDead=true for exited process")
	}
	if result.Escalated {
		t.Error("expected Escalated=false for exited process")
	}
}

func TestTerminateProcess_GracefulShutdown(t *testing.T) {
	// Start a process that exits on SIGTERM (default behavior).
	pid := startAndReap(t, exec.Command("sleep", "60"))

	result, err := TerminateProcess(pid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyDead {
		t.Error("expected AlreadyDead=false")
	}
	if result.Escalated {
		t.Error("expected Escalated=false for process that handles SIGTERM")
	}
	if !result.Signaled {
		t.Error("expected Signaled=true")
	}
}

func TestTerminateProcess_Escalation(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping escalation test in CI (requires signal-trapping subprocess)")
	}

	// Start a subprocess that traps SIGTERM and ignores it, forcing SIGKILL escalation.
	pid := startAndReap(t, exec.Command("bash", "-c", "trap '' TERM; sleep 60"))

	// Give the shell time to set up the trap.
	time.Sleep(200 * time.Millisecond)

	if !isProcessAlive(pid) {
		t.Fatal("subprocess should be alive before TerminateProcess")
	}

	result, err := TerminateProcess(pid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyDead {
		t.Error("expected AlreadyDead=false")
	}
	if !result.Escalated {
		t.Error("expected Escalated=true when process ignores SIGTERM")
	}
	if !result.Signaled {
		t.Error("expected Signaled=true")
	}
}

func TestTerminateProcess_InvalidPID(t *testing.T) {
	// PID 4999999 is extremely unlikely to exist.
	result, err := TerminateProcess(4999999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AlreadyDead {
		t.Error("expected AlreadyDead=true for non-existent PID")
	}
}
