package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withStdinPipe replaces os.Stdin with the read end of a pipe carrying
// data, restoring the original os.Stdin on cleanup. This simulates how
// Claude Code / Codex invoke a hook command: stdin is a pipe carrying a
// JSON payload, not the interactive terminal.
func withStdinPipe(t *testing.T, data string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = original
		r.Close()
	})
}

func TestSessionIDFromStdinReadsJSONPipe(t *testing.T) {
	withStdinPipe(t, `{"session_id":"sess-abc","cwd":"/tmp"}`)
	if got := sessionIDFromStdin(); got != "sess-abc" {
		t.Fatalf("sessionIDFromStdin() = %q, want %q", got, "sess-abc")
	}
}

func TestSessionIDFromStdinEmptyOnNonJSON(t *testing.T) {
	withStdinPipe(t, "not json")
	if got := sessionIDFromStdin(); got != "" {
		t.Fatalf("sessionIDFromStdin() = %q, want empty on invalid JSON", got)
	}
}

func TestSessionIDFromStdinEmptyOnEmptyPipe(t *testing.T) {
	withStdinPipe(t, "")
	if got := sessionIDFromStdin(); got != "" {
		t.Fatalf("sessionIDFromStdin() = %q, want empty on empty stdin", got)
	}
}

func TestSessionIDFromStdinSkipsCharDevice(t *testing.T) {
	// /dev/null is a character device, same as a real terminal or the
	// stdin Go's exec.Command gives a child by default. This must never
	// be treated as a hook payload pipe.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	original := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = original }()

	if got := sessionIDFromStdin(); got != "" {
		t.Fatalf("sessionIDFromStdin() = %q, want empty for a char device", got)
	}
}

func TestResolveIDPriorityOrder(t *testing.T) {
	t.Run("explicit wins over everything", func(t *testing.T) {
		t.Setenv("OPEN_SPINNER_ID", "env-id")
		if got := resolveID("explicit-id", "agent"); got != "explicit-id" {
			t.Fatalf("resolveID() = %q, want explicit-id", got)
		}
	})

	t.Run("env wins over stdin session id", func(t *testing.T) {
		withStdinPipe(t, `{"session_id":"sess-xyz"}`)
		t.Setenv("OPEN_SPINNER_ID", "env-id")
		if got := resolveID("", "agent"); got != "env-id" {
			t.Fatalf("resolveID() = %q, want env-id", got)
		}
	})

	t.Run("stdin session id wins over tty and agent", func(t *testing.T) {
		withStdinPipe(t, `{"session_id":"sess-xyz"}`)
		t.Setenv("OPEN_SPINNER_TTY", "/dev/ttys009")
		if got := resolveID("", "agent"); got != "sess-xyz" {
			t.Fatalf("resolveID() = %q, want sess-xyz", got)
		}
	})

	t.Run("tty wins over bare agent name", func(t *testing.T) {
		withStdinPipe(t, "")
		t.Setenv("OPEN_SPINNER_TTY", "/dev/ttys009")
		if got := resolveID("", "agent"); got != "/dev/ttys009" {
			t.Fatalf("resolveID() = %q, want /dev/ttys009", got)
		}
	})

	t.Run("bare agent name is the last resort", func(t *testing.T) {
		withStdinPipe(t, "")
		if got := resolveID("", "agent"); got != "agent" {
			t.Fatalf("resolveID() = %q, want agent", got)
		}
	})
}

// TestSetBusyIsolatesConcurrentSessionsOfSameAgent is the regression test
// for the actual reported bug: the spinner never stopped because every
// hook invocation for the same --agent collapsed onto one shared status
// file, so one tab's activity kept re-arming a different, already-idle
// tab. Two sessions of "claude" with distinct stdin session_ids (as Claude
// Code/Codex genuinely provide) must land in two independent status
// files, and idling one must not disturb the other's derived state.
func TestSetBusyIsolatesConcurrentSessionsOfSameAgent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_STATUS_DIR", dir)

	withStdinPipe(t, `{"session_id":"session-A"}`)
	if err := setCmd([]string{"busy", "--agent", "claude"}); err != nil {
		t.Fatal(err)
	}

	withStdinPipe(t, `{"session_id":"session-B"}`)
	if err := setCmd([]string{"busy", "--agent", "claude"}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var jsonFiles int
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsonFiles++
		}
	}
	if jsonFiles != 2 {
		t.Fatalf("expected 2 independent status files for 2 sessions of the same agent, got %d", jsonFiles)
	}

	// Session A goes idle; session B must be unaffected.
	withStdinPipe(t, `{"session_id":"session-A"}`)
	if err := setCmd([]string{"idle", "--agent", "claude"}); err != nil {
		t.Fatal(err)
	}

	statuses, err := readStatuses(dir, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	for _, s := range statuses {
		byID[s.ID] = s.State
	}
	if byID["session-A"] != "idle" {
		t.Fatalf("session-A state = %q, want idle", byID["session-A"])
	}
	if byID["session-B"] != "busy" {
		t.Fatalf("session-B state = %q, want busy (must be unaffected by session-A going idle)", byID["session-B"])
	}
}
