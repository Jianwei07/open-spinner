package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// buildCLI compiles the real binary once so the fake-agent script below can
// exercise the full store -> hook -> auto-spawned-renderer -> tty chain,
// including the detached background render process, which `go run` can't
// do cleanly.
func buildCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "open-spinner")
	cmd := exec.Command("go", "build", "-o", bin, "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func readTitlesFromMaster(t *testing.T, r *os.File, minCount int, timeout time.Duration) []string {
	t.Helper()
	var titles []string
	var buf []byte
	tmp := make([]byte, 256)
	deadline := time.Now().Add(timeout)
	const oscPrefix = "\x1b]0;"

	for len(titles) < minCount && time.Now().Before(deadline) {
		_ = r.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				start := bytes.Index(buf, []byte(oscPrefix))
				if start < 0 {
					break
				}
				end := bytes.IndexByte(buf[start:], '\a')
				if end < 0 {
					break
				}
				end += start
				titles = append(titles, string(buf[start+len(oscPrefix):end]))
				buf = buf[end+1:]
			}
		}
		if err != nil {
			continue
		}
	}
	return titles
}

// TestFakeAgentLifecycleDrivesRendererAcrossPTY replays the exact sequence
// of CLI calls a real hook config fires (see install.go's claudeEventVerbs
// / codexEventVerbs): UserPromptSubmit -> busy, Notification -> attention,
// Stop -> idle, then session end -> clear. No real agent or API key is
// involved; this proves the whole chain (store write -> auto-spawned
// renderer -> OSC bytes on a real tty -> clean shutdown) end-to-end.
func TestFakeAgentLifecycleDrivesRendererAcrossPTY(t *testing.T) {
	bin := buildCLI(t)
	statusDir := t.TempDir()

	ptyMaster, ptySlave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptyMaster.Close()
	defer ptySlave.Close()

	const sessionID = "fake-agent-session"
	env := append(os.Environ(),
		"AGENT_STATUS_DIR="+statusDir,
		"OPEN_SPINNER_TTY="+ptySlave.Name(),
	)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	// UserPromptSubmit hook fires -> busy, renderer auto-spawns and animates.
	run("set", "busy", "--agent", "fakeagent", "--id", sessionID)
	busyFrames := readTitlesFromMaster(t, ptyMaster, 3, time.Second)
	if len(busyFrames) < 3 {
		t.Fatalf("expected the auto-spawned renderer to animate while busy, got frames: %v", busyFrames)
	}
	unique := map[string]bool{}
	for _, f := range busyFrames {
		unique[f] = true
	}
	if len(unique) < 2 {
		t.Fatalf("busy frames did not vary, renderer looks stuck: %v", busyFrames)
	}

	// Notification hook fires (e.g. permission prompt) -> steady attention.
	run("set", "attention", "--agent", "fakeagent", "--id", sessionID, "--text", "approval needed")
	attentionFrames := readTitlesFromMaster(t, ptyMaster, 1, time.Second)
	sawAttention := false
	for _, f := range attentionFrames {
		if strings.Contains(f, "⚠") {
			sawAttention = true
		}
	}
	if !sawAttention {
		t.Fatalf("expected a steady attention glyph after Notification hook, got %v", attentionFrames)
	}

	// Stop hook fires -> idle, then the session ends entirely -> clear.
	run("set", "idle", "--agent", "fakeagent", "--id", sessionID)
	run("clear", "--id", sessionID)

	// The renderer should notice the clear (no grace period for explicit
	// clear) and exit, restoring/clearing the title on its way out.
	deadline := time.Now().Add(2 * time.Second)
	sawRestore := false
	for time.Now().Before(deadline) {
		frames := readTitlesFromMaster(t, ptyMaster, 1, 200*time.Millisecond)
		if len(frames) > 0 {
			sawRestore = true
			break
		}
	}
	if !sawRestore {
		t.Fatal("expected the renderer to write a final restore/clear title after session clear")
	}
}
