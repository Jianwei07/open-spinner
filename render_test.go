package main

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// fakeWriter is a deterministic stand-in for a tty file descriptor. It lets
// the lifecycle tests (idle grace, explicit clear, write failure, signal
// restore) assert exact behavior without depending on OS-specific PTY
// close/EIO timing, which varies across platforms and would make CI
// flaky. The animation test below still goes through a real PTY, since
// that's the one property (bytes actually reaching a tty) a fake can't
// prove.
type fakeWriter struct {
	mu        sync.Mutex
	writes    [][]byte
	failAfter int // -1 == never fail
}

func newFakeWriter(failAfter int) *fakeWriter {
	return &fakeWriter{failAfter: failAfter}
}

func (f *fakeWriter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAfter >= 0 && len(f.writes) >= f.failAfter {
		return 0, errors.New("simulated closed tty")
	}
	cp := append([]byte(nil), p...)
	f.writes = append(f.writes, cp)
	return len(p), nil
}

func (f *fakeWriter) all() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.writes))
	for i, w := range f.writes {
		out[i] = string(w)
	}
	return out
}

func (f *fakeWriter) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) == 0 {
		return ""
	}
	return string(f.writes[len(f.writes)-1])
}

func baseRenderConfig(dir, id string) renderConfig {
	return renderConfig{
		id:        id,
		tty:       "fake",
		animate:   true,
		interval:  10 * time.Millisecond,
		idleGrace: 80 * time.Millisecond,
		statusDir: dir,
	}
}

func runLoopWithTimeout(t *testing.T, cfg renderConfig, w writeCloser, sigCh <-chan os.Signal, timeout time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- runRenderLoop(cfg, w, sigCh)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		t.Fatal("runRenderLoop did not return before timeout")
		return nil
	}
}

func TestRenderLoopAnimatesWhileBusyOverRealPTY(t *testing.T) {
	dir := t.TempDir()
	ptyMaster, ptySlave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptyMaster.Close()
	defer ptySlave.Close()

	const id = "pane-anim"
	status := Status{
		V: 1, ID: id, Agent: "claude", State: "busy",
		UpdatedAt: time.Now().UTC(), TTLMS: time.Minute.Milliseconds(),
	}
	if err := writeStatus(dir, status); err != nil {
		t.Fatal(err)
	}

	cfg := renderConfig{
		id: id, tty: ptySlave.Name(), animate: true,
		interval: 15 * time.Millisecond, idleGrace: time.Second, statusDir: dir,
	}
	sigCh := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- runRenderLoop(cfg, ptySlave, sigCh) }()

	frames := readTitlesFromPTY(t, ptyMaster, 4, 800*time.Millisecond)
	unique := map[string]bool{}
	for _, f := range frames {
		unique[f] = true
	}
	if len(unique) < 3 {
		t.Fatalf("expected >=3 distinct animation frames while busy, got %v", frames)
	}

	sigCh <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runRenderLoop returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runRenderLoop did not exit after signal")
	}
}

func readTitlesFromPTY(t *testing.T, r *os.File, minCount int, timeout time.Duration) []string {
	t.Helper()
	var titles []string
	var buf []byte
	tmp := make([]byte, 256)
	deadline := time.Now().Add(timeout)

	for len(titles) < minCount && time.Now().Before(deadline) {
		_ = r.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				start := bytes.Index(buf, []byte(oscTitlePrefix))
				if start < 0 {
					break
				}
				end := bytes.IndexByte(buf[start:], '\a')
				if end < 0 {
					break
				}
				end += start
				titles = append(titles, string(buf[start+len(oscTitlePrefix):end]))
				buf = buf[end+1:]
			}
		}
		if err != nil {
			continue // read-deadline timeouts are expected between ticks
		}
	}
	return titles
}

func TestRenderLoopExitsAfterIdleGrace(t *testing.T) {
	dir := t.TempDir()
	const id = "pane-idle"
	status := Status{V: 1, ID: id, Agent: "codex", State: "idle", UpdatedAt: time.Now().UTC(), TTLMS: time.Minute.Milliseconds()}
	if err := writeStatus(dir, status); err != nil {
		t.Fatal(err)
	}

	cfg := baseRenderConfig(dir, id)
	cfg.restore = "original-title"
	w := newFakeWriter(-1)
	sigCh := make(chan os.Signal, 1)

	err := runLoopWithTimeout(t, cfg, w, sigCh, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := w.last(); got != string(oscTitle("original-title")) {
		t.Fatalf("final write = %q, want restore title %q", got, string(oscTitle("original-title")))
	}
}

func TestRenderLoopExitsImmediatelyOnExplicitClear(t *testing.T) {
	dir := t.TempDir() // no status ever written == "cleared"
	cfg := baseRenderConfig(dir, "pane-cleared")
	cfg.restore = "shell-prompt"
	cfg.idleGrace = 10 * time.Second // long grace, proves clear skips it entirely
	w := newFakeWriter(-1)
	sigCh := make(chan os.Signal, 1)

	started := time.Now()
	err := runLoopWithTimeout(t, cfg, w, sigCh, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("explicit clear took %v to exit, want near-immediate (well under idle-grace)", elapsed)
	}
	if got := w.last(); got != string(oscTitle("shell-prompt")) {
		t.Fatalf("final write = %q, want restore title", got)
	}
}

func TestRenderLoopExitsOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	const id = "pane-closed"
	status := Status{V: 1, ID: id, Agent: "opencode", State: "busy", UpdatedAt: time.Now().UTC(), TTLMS: time.Minute.Milliseconds()}
	if err := writeStatus(dir, status); err != nil {
		t.Fatal(err)
	}

	cfg := baseRenderConfig(dir, id)
	w := newFakeWriter(0) // fails on the very first write, simulating a closed tab
	sigCh := make(chan os.Signal, 1)

	err := runLoopWithTimeout(t, cfg, w, sigCh, time.Second)
	if err != nil {
		t.Fatalf("expected clean nil return on write failure (not a hard error), got %v", err)
	}
}

func TestRenderLoopRestoresTitleOnSignal(t *testing.T) {
	dir := t.TempDir()
	const id = "pane-signal"
	status := Status{V: 1, ID: id, Agent: "claude", State: "busy", UpdatedAt: time.Now().UTC(), TTLMS: time.Minute.Milliseconds()}
	if err := writeStatus(dir, status); err != nil {
		t.Fatal(err)
	}

	cfg := baseRenderConfig(dir, id)
	cfg.restore = "zsh %~"
	w := newFakeWriter(-1)
	sigCh := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() { done <- runRenderLoop(cfg, w, sigCh) }()
	time.Sleep(30 * time.Millisecond) // let at least one busy frame render first
	sigCh <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runRenderLoop did not exit after signal")
	}
	if got := w.last(); got != string(oscTitle("zsh %~")) {
		t.Fatalf("final write = %q, want restore title %q", got, string(oscTitle("zsh %~")))
	}
}

func TestAcquireTTYLockIsSingleInstancePerTTY(t *testing.T) {
	dir := t.TempDir()
	const tty = "/dev/ttys999-fake"

	lock1, acquired1, err := acquireTTYLock(dir, tty)
	if err != nil || !acquired1 {
		t.Fatalf("first acquire: acquired=%v err=%v", acquired1, err)
	}

	_, acquired2, err := acquireTTYLock(dir, tty)
	if err != nil {
		t.Fatalf("second acquire returned error: %v", err)
	}
	if acquired2 {
		t.Fatal("second acquire for same tty succeeded while first holder is alive; expected no-op")
	}

	releaseTTYLock(lock1)

	lock3, acquired3, err := acquireTTYLock(dir, tty)
	if err != nil || !acquired3 {
		t.Fatalf("acquire after release: acquired=%v err=%v", acquired3, err)
	}
	releaseTTYLock(lock3)
}

func TestAcquireTTYLockReclaimsStaleLock(t *testing.T) {
	dir := t.TempDir()
	const tty = "/dev/ttys998-fake"

	// Simulate a lock left behind by a renderer that no longer exists by
	// writing a pid that is guaranteed not to be alive.
	lockDir := dir + "/render-locks"
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stalePath := lockDir + "/" + safeName(tty) + ".lock"
	if err := os.WriteFile(stalePath, []byte("999999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, acquired, err := acquireTTYLock(dir, tty)
	if err != nil || !acquired {
		t.Fatalf("expected stale lock to be reclaimed, acquired=%v err=%v", acquired, err)
	}
	releaseTTYLock(lock)
}
