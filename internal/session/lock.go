package session

import (
	"fmt"
	"os"
	"syscall"
)

// withFileLock acquires an exclusive flock on a .lock file adjacent to path,
// executes fn, then releases the lock. This prevents concurrent read-modify-write
// races on sessions.yaml.
func withFileLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire file lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	return fn()
}
