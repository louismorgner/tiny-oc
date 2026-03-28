package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// setupTempSessionsFile creates a temp directory with an empty sessions.yaml
// and returns the path. Caller must clean up.
func setupTempSessionsFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.yaml")
	if err := os.WriteFile(path, []byte("sessions: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWithFileLock_MutualExclusion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	counter := 0
	var wg sync.WaitGroup
	iterations := 50

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withFileLock(path, func() error {
				// Read-modify-write: without locking this would lose increments
				val := counter
				// Small sleep to increase chance of interleaving
				time.Sleep(time.Microsecond)
				counter = val + 1
				return nil
			})
			if err != nil {
				t.Errorf("withFileLock failed: %v", err)
			}
		}()
	}

	wg.Wait()

	if counter != iterations {
		t.Errorf("expected counter=%d, got %d (indicates race condition)", iterations, counter)
	}
}

func TestWithFileLock_ConcurrentSessionAdd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.yaml")
	if err := os.WriteFile(path, []byte("sessions: []\n"), 0600); err != nil {
		t.Fatal(err)
	}

	numWriters := 20
	var wg sync.WaitGroup

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := withFileLock(path, func() error {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				var sf SessionsFile
				_ = yaml.Unmarshal(data, &sf)
				sf.Sessions = append(sf.Sessions, Session{
					ID:    "session-" + strconv.Itoa(idx),
					Agent: "test-agent",
				})
				out, err := yaml.Marshal(sf)
				if err != nil {
					return err
				}
				return os.WriteFile(path, out, 0600)
			})
			if err != nil {
				t.Errorf("concurrent add failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all sessions were written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sf SessionsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		t.Fatal(err)
	}

	if len(sf.Sessions) != numWriters {
		t.Errorf("expected %d sessions, got %d (concurrent writes lost data)", numWriters, len(sf.Sessions))
	}
}

func TestResolvedStatus_Stale(t *testing.T) {
	s := &Session{
		WorkspacePath: "/nonexistent/path/that/does/not/exist",
		Status:        StatusActive,
	}
	if got := s.ResolvedStatus(); got != "stale" {
		t.Errorf("expected 'stale', got %q", got)
	}
}

func TestResolvedStatus_ActiveInteractive(t *testing.T) {
	dir := t.TempDir()
	s := &Session{
		WorkspacePath: dir,
		Status:        StatusActive,
	}
	if got := s.ResolvedStatus(); got != "active" {
		t.Errorf("expected 'active', got %q", got)
	}
}

func TestResolvedStatus_CompletedInteractive(t *testing.T) {
	dir := t.TempDir()
	s := &Session{
		WorkspacePath: dir,
		Status:        StatusCompleted,
	}
	if got := s.ResolvedStatus(); got != "completed" {
		t.Errorf("expected 'completed', got %q", got)
	}
}

func TestResolvedStatus_SubAgent_CompletedSuccess(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "toc-output.txt"), []byte("done"), 0644)
	os.WriteFile(filepath.Join(dir, "toc-exit-code.txt"), []byte("0"), 0644)

	s := &Session{
		WorkspacePath:   dir,
		Status:          StatusActive,
		ParentSessionID: "parent-123",
	}
	if got := s.ResolvedStatus(); got != StatusCompletedOK {
		t.Errorf("expected %q, got %q", StatusCompletedOK, got)
	}
}

func TestResolvedStatus_SubAgent_CompletedError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "toc-output.txt"), []byte("error output"), 0644)
	os.WriteFile(filepath.Join(dir, "toc-exit-code.txt"), []byte("1"), 0644)

	s := &Session{
		WorkspacePath:   dir,
		Status:          StatusActive,
		ParentSessionID: "parent-123",
	}
	if got := s.ResolvedStatus(); got != StatusCompletedError {
		t.Errorf("expected %q, got %q", StatusCompletedError, got)
	}
}

func TestResolvedStatus_SubAgent_CompletedLegacy(t *testing.T) {
	// Legacy session: output exists but no exit code file
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "toc-output.txt"), []byte("done"), 0644)

	s := &Session{
		WorkspacePath:   dir,
		Status:          StatusActive,
		ParentSessionID: "parent-123",
	}
	if got := s.ResolvedStatus(); got != "completed" {
		t.Errorf("expected 'completed' (legacy fallback), got %q", got)
	}
}

func TestResolvedStatus_SubAgent_Cancelled(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "toc-cancelled.txt"), []byte("cancelled"), 0644)

	s := &Session{
		WorkspacePath:   dir,
		Status:          StatusActive,
		ParentSessionID: "parent-123",
	}
	if got := s.ResolvedStatus(); got != StatusCancelled {
		t.Errorf("expected %q, got %q", StatusCancelled, got)
	}
}

func TestResolvedStatus_SubAgent_Zombie(t *testing.T) {
	dir := t.TempDir()
	// Write a PID that definitely doesn't exist (PID 1 is init, use a very high PID)
	os.WriteFile(filepath.Join(dir, "toc-pid.txt"), []byte("4999999"), 0644)

	s := &Session{
		WorkspacePath:   dir,
		Status:          StatusActive,
		ParentSessionID: "parent-123",
	}
	if got := s.ResolvedStatus(); got != StatusZombie {
		t.Errorf("expected %q, got %q", StatusZombie, got)
	}
}

func TestResolvedStatus_SubAgent_ActiveNoPID(t *testing.T) {
	// No output, no PID file — legacy active session
	dir := t.TempDir()
	s := &Session{
		WorkspacePath:   dir,
		Status:          StatusActive,
		ParentSessionID: "parent-123",
	}
	if got := s.ResolvedStatus(); got != "active" {
		t.Errorf("expected 'active', got %q", got)
	}
}

func TestResolvedStatus_SubAgent_ActiveWithLivePID(t *testing.T) {
	dir := t.TempDir()
	// Use our own PID which is guaranteed to be alive
	os.WriteFile(filepath.Join(dir, "toc-pid.txt"), []byte(strconv.Itoa(os.Getpid())), 0644)

	s := &Session{
		WorkspacePath:   dir,
		Status:          StatusActive,
		ParentSessionID: "parent-123",
	}
	if got := s.ResolvedStatus(); got != "active" {
		t.Errorf("expected 'active', got %q", got)
	}
}

func TestReadPID(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "toc-pid.txt"), []byte("12345\n"), 0644)

	s := &Session{WorkspacePath: dir}
	pid, err := s.ReadPID()
	if err != nil {
		t.Fatal(err)
	}
	if pid != 12345 {
		t.Errorf("expected PID 12345, got %d", pid)
	}
}

func TestReadPID_Missing(t *testing.T) {
	dir := t.TempDir()
	s := &Session{WorkspacePath: dir}
	_, err := s.ReadPID()
	if err == nil {
		t.Error("expected error for missing PID file")
	}
}

func TestReadExitCode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "toc-exit-code.txt"), []byte("42\n"), 0644)

	s := &Session{WorkspacePath: dir}
	code, err := s.ReadExitCode()
	if err != nil {
		t.Fatal(err)
	}
	if code != 42 {
		t.Errorf("expected exit code 42, got %d", code)
	}
}

func TestReadExitCode_Zero(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "toc-exit-code.txt"), []byte("0"), 0644)

	s := &Session{WorkspacePath: dir}
	code, err := s.ReadExitCode()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestIsProcessAlive_Self(t *testing.T) {
	if !isProcessAlive(os.Getpid()) {
		t.Error("expected own PID to be alive")
	}
}

func TestIsProcessAlive_Dead(t *testing.T) {
	// PID 4999999 is extremely unlikely to be in use
	if isProcessAlive(4999999) {
		t.Error("expected PID 4999999 to be dead")
	}
}

func TestStopByPrefix_FindsPIDFile(t *testing.T) {
	// Simulate the toc stop <prefix> flow: resolve prefix → read PID.
	// This was broken because interactive sessions didn't write PID files,
	// so ReadPID always failed regardless of prefix or full ID.
	workDir := t.TempDir()

	fullID := "f74e0d70-abcd-1234-5678-123456789abc"
	wantPID := os.Getpid() // use own PID as a known-alive process

	// Write PID file like LaunchInteractive now does.
	if err := os.WriteFile(filepath.Join(workDir, "toc-pid.txt"), []byte(strconv.Itoa(wantPID)), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a sessions.yaml with one session.
	sf := &SessionsFile{Sessions: []Session{{
		ID:            fullID,
		Agent:         "test-agent",
		WorkspacePath: workDir,
		Status:        StatusActive,
		CreatedAt:     time.Now(),
	}}}
	data, err := yaml.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}

	// FindByIDPrefixInWorkspace lets us test prefix resolution + PID read
	// without changing CWD (which FindByIDPrefix requires).
	wsDir := t.TempDir()
	tocDir := filepath.Join(wsDir, ".toc")
	if err := os.MkdirAll(tocDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tocDir, "sessions.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}

	// Use an 8-char prefix (the short form shown in toc output).
	prefix := fullID[:8]

	s, err := FindByIDPrefixInWorkspace(wsDir, prefix)
	if err != nil {
		t.Fatalf("FindByIDPrefixInWorkspace(%q): %v", prefix, err)
	}

	if s.ID != fullID {
		t.Errorf("expected full ID %q, got %q", fullID, s.ID)
	}

	pid, err := s.ReadPID()
	if err != nil {
		t.Fatalf("ReadPID failed after prefix resolution: %v", err)
	}
	if pid != wantPID {
		t.Errorf("expected PID %d, got %d", wantPID, pid)
	}
}

func TestStopByPrefix_AmbiguousPrefix(t *testing.T) {
	wsDir := t.TempDir()
	tocDir := filepath.Join(wsDir, ".toc")
	if err := os.MkdirAll(tocDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Two sessions sharing the same 8-char prefix.
	sf := &SessionsFile{Sessions: []Session{
		{ID: "f74e0d70-aaaa-0000-0000-000000000001", Agent: "a", WorkspacePath: t.TempDir(), Status: StatusActive},
		{ID: "f74e0d70-bbbb-0000-0000-000000000002", Agent: "b", WorkspacePath: t.TempDir(), Status: StatusActive},
	}}
	data, err := yaml.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tocDir, "sessions.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}

	_, err = FindByIDPrefixInWorkspace(wsDir, "f74e0d70")
	if err == nil {
		t.Fatal("expected ambiguous prefix error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got: %v", err)
	}
}
