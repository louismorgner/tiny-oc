package session

import (
	"fmt"
	"syscall"
	"time"
)

// TerminateResult describes the outcome of a process termination attempt.
type TerminateResult struct {
	// Signaled is true if a signal was sent to the process.
	Signaled bool
	// AlreadyDead is true if the process was not running.
	AlreadyDead bool
	// Escalated is true if SIGKILL was needed after SIGTERM.
	Escalated bool
}

// TerminateProcess sends SIGTERM to the process group, waits briefly, then
// sends SIGKILL if the process is still alive. Returns a result describing
// what happened.
func TerminateProcess(pid int) (*TerminateResult, error) {
	if !isProcessAlive(pid) {
		return &TerminateResult{AlreadyDead: true}, nil
	}

	// Try SIGTERM on the process group first (negative PID).
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		// Group kill failed — try the process directly.
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			if err == syscall.ESRCH {
				return &TerminateResult{AlreadyDead: true}, nil
			}
			return nil, fmt.Errorf("failed to send SIGTERM to process %d: %w", pid, err)
		}
	}

	// Wait up to 3 seconds for graceful shutdown.
	for i := 0; i < 6; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isProcessAlive(pid) {
			return &TerminateResult{Signaled: true}, nil
		}
	}

	// Still alive — escalate to SIGKILL.
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			if err == syscall.ESRCH {
				return &TerminateResult{Signaled: true}, nil
			}
			return nil, fmt.Errorf("failed to send SIGKILL to process %d: %w", pid, err)
		}
	}

	return &TerminateResult{Signaled: true, Escalated: true}, nil
}
