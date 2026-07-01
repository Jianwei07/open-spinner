package main

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultIdleGrace = 2 * time.Second
const defaultRenderInterval = 100 * time.Millisecond

// renderConfig holds everything one renderCmd/renderLoop instance needs,
// pulled out of flag parsing so tests can construct it directly.
type renderConfig struct {
	id        string
	tty       string
	restore   string
	animate   bool
	interval  time.Duration
	idleGrace time.Duration
	statusDir string
}

func renderCmd(args []string) error {
	fs := newFlagSet("render")
	id := fs.String("id", "", "status id to monitor")
	ttyFlag := fs.String("tty", "", "tty device to render the title into")
	noAnim := fs.Bool("no-anim", false, "disable animation, static glyph only")
	restore := fs.String("restore", "", "title to restore on exit (best-effort; empty clears the title)")
	interval := fs.Duration("interval", defaultRenderInterval, "animation frame interval")
	idleGrace := fs.Duration("idle-grace", defaultIdleGrace, "how long to keep rendering after going idle before exiting")
	if err := fs.Parse(args); err != nil {
		return err
	}

	resolvedID := resolveID(*id, "")
	if resolvedID == "" {
		return errors.New("render requires --id (or OPEN_SPINNER_ID/AGENT_STATUS_ID/TMUX_PANE)")
	}

	tty := *ttyFlag
	if tty == "" {
		tty = resolveTTY()
	}
	if tty == "" {
		return errors.New("render requires --tty (or a resolvable controlling terminal)")
	}

	cfg := renderConfig{
		id:        resolvedID,
		tty:       tty,
		restore:   *restore,
		animate:   animationEnabled(*noAnim, os.Getenv("OPEN_SPINNER_ANIM"), os.Getenv("TMUX")),
		interval:  *interval,
		idleGrace: *idleGrace,
		statusDir: statusDir(),
	}

	lock, acquired, err := acquireTTYLock(cfg.statusDir, cfg.tty)
	if err != nil {
		return err
	}
	if !acquired {
		// Another renderer already owns this tty. Hooks fire repeatedly
		// (every prompt submit, every tool call); only the first spawn
		// should win. This is a deliberate no-op, not an error.
		return nil
	}
	defer releaseTTYLock(lock)

	ttyFile, err := os.OpenFile(cfg.tty, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer ttyFile.Close()

	sigCh := newSignalChannel()
	defer stopSignalChannel(sigCh)

	return runRenderLoop(cfg, ttyFile, sigCh)
}

// runRenderLoop is the testable core: given an already-open tty writer and
// a signal channel, it ticks until the status goes away/idle past grace,
// the tty write fails (tab closed), or it's signaled to stop. Split out
// from renderCmd so integration tests can point it at a PTY slave without
// spawning a subprocess or fighting over lockfiles.
func runRenderLoop(cfg renderConfig, ttyFile writeCloser, sigCh <-chan os.Signal) error {
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	restoreAndExit := func() {
		writeOSCTitle(ttyFile, cfg.restore)
	}

	var lastText string
	var idleSince time.Time
	tick := 0

	write := func(text string) error {
		if text == lastText {
			return nil
		}
		if _, err := ttyFile.Write(oscTitle(text)); err != nil {
			return err
		}
		lastText = text
		return nil
	}

	for {
		select {
		case <-sigCh:
			restoreAndExit()
			return nil
		case <-ticker.C:
			tick++
			statuses, err := readStatuses(cfg.statusDir, time.Now().UTC())
			if err != nil {
				restoreAndExit()
				return err
			}

			current := findStatus(statuses, cfg.id)
			if current == nil {
				// Explicit clear: no grace period, exit right away.
				restoreAndExit()
				return nil
			}

			if current.State == "idle" || current.State == "stale" {
				if idleSince.IsZero() {
					idleSince = time.Now()
				}
				if time.Since(idleSince) >= cfg.idleGrace {
					restoreAndExit()
					return nil
				}
				text, _ := glyphForState(current.State, current.Agent, tick, false)
				if err := write(text); err != nil {
					restoreAndExit()
					return nil
				}
				continue
			}

			idleSince = time.Time{}
			text, _ := glyphForState(current.State, current.Agent, tick, cfg.animate)
			if err := write(text); err != nil {
				restoreAndExit()
				return nil
			}
		}
	}
}

func writeOSCTitle(w writeCloser, title string) {
	_, _ = w.Write(oscTitle(title))
}

func findStatus(statuses []Status, id string) *Status {
	for i := range statuses {
		if statuses[i].ID == id {
			return &statuses[i]
		}
	}
	return nil
}

// writeCloser is the minimal surface runRenderLoop needs from a tty
// handle, so tests can pass the write end of a PTY pair instead of an
// *os.File opened by path.
type writeCloser interface {
	Write(p []byte) (int, error)
}

// maybeSpawnRenderer is called from setCmd on every busy write. It always
// attempts to spawn: the child renderer itself checks the tty lockfile
// and exits immediately as a no-op if one is already running, so the
// caller never needs its own coordination logic.
func maybeSpawnRenderer(status Status) {
	if os.Getenv("OPEN_SPINNER_NO_RENDER") == "1" {
		return
	}
	if status.TTY == "" {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}

	args := []string{
		"render",
		"--id", status.ID,
		"--tty", status.TTY,
	}

	proc, err := os.StartProcess(exe, append([]string{exe}, args...), &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{nil, nil, nil},
		Sys:   detachedSysProcAttr(),
	})
	if err != nil {
		return
	}
	// Detach: we don't wait on it, the renderer manages its own lifetime.
	_ = proc.Release()
}

func acquireTTYLock(dir, tty string) (*os.File, bool, error) {
	lockDir := filepath.Join(dir, "render-locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, false, err
	}
	lockPath := filepath.Join(lockDir, safeName(tty)+".lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			if lockHolderAlive(lockPath) {
				return nil, false, nil
			}
			// Stale lock left by a crashed/killed renderer; reclaim it.
			if rmErr := os.Remove(lockPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return nil, false, rmErr
			}
			return acquireTTYLock(dir, tty)
		}
		return nil, false, err
	}

	if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		f.Close()
		os.Remove(lockPath)
		return nil, false, err
	}
	return f, true, nil
}

func releaseTTYLock(f *os.File) {
	name := f.Name()
	f.Close()
	os.Remove(name)
}

// detachedSysProcAttr puts the spawned renderer in its own session so it
// survives the hook process (and its shell) exiting.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func newSignalChannel() chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}

func stopSignalChannel(ch chan os.Signal) {
	signal.Stop(ch)
}

func lockHolderAlive(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	// Signal 0 does no harm; it only checks whether the process exists
	// and we have permission to signal it.
	return syscall.Kill(pid, 0) == nil
}
