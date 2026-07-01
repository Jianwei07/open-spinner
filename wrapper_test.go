package main

import (
	"testing"
	"time"
)

// TestRunWrapperCmdWritesStatusWithNoTTL is the regression test for a real
// bug: the busy status `run` writes once at spawn used a fixed 5-minute
// TTL but is never refreshed for the life of the wrapped process (unlike
// a hook-driven status, which gets rewritten every turn). Any wrapped run
// longer than 5 minutes would derive as "stale" — which the renderer
// treats the same as idle — and restore the tab title while the agent was
// still working. `run` already removes the status explicitly on exit, so
// no TTL is needed as a safety net.
func TestRunWrapperCmdWritesStatusWithNoTTL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_STATUS_DIR", dir)
	t.Setenv("OPEN_SPINNER_NO_RENDER", "1")
	t.Setenv("OPEN_SPINNER_ID", "wrapper-ttl-test")

	done := make(chan error, 1)
	go func() {
		done <- runWrapperCmd([]string{"--", "sleep", "0.3"})
	}()

	var status *Status
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statuses, err := readStatuses(dir, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if s := findStatus(statuses, "wrapper-ttl-test"); s != nil {
			status = s
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status == nil {
		t.Fatal("expected to observe the wrapper's busy status while the wrapped process ran")
	}
	if status.TTLMS != 0 {
		t.Fatalf("expected zero TTL on a run-wrapper status (never refreshed mid-run), got %d", status.TTLMS)
	}

	if err := <-done; err != nil {
		t.Fatalf("runWrapperCmd: %v", err)
	}
}
