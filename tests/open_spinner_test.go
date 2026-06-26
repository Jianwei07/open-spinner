package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type status struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	State string `json:"state"`
	Text  string `json:"text"`
}

func TestSetListAndPrintFormats(t *testing.T) {
	dir := t.TempDir()

	runCLI(t, dir, "set", "busy", "--agent", "opencode", "--text", "running tool", "--id", "pane-1", "--ttl", "5m")

	statuses := listStatuses(t, dir)
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if statuses[0].ID != "pane-1" || statuses[0].Agent != "opencode" || statuses[0].State != "busy" || statuses[0].Text != "running tool" {
		t.Fatalf("unexpected status: %#v", statuses[0])
	}

	plain := strings.TrimSpace(runCLI(t, dir, "print", "--format", "plain"))
	if plain != "opencode busy: running tool" {
		t.Fatalf("plain output = %q", plain)
	}

	tmux := strings.TrimSpace(runCLI(t, dir, "print", "--format", "tmux"))
	if tmux != "opencode:busy" {
		t.Fatalf("tmux output = %q", tmux)
	}

	var printed []status
	decodeJSON(t, runCLI(t, dir, "print", "--format", "json"), &printed)
	if len(printed) != 1 || printed[0].State != "busy" {
		t.Fatalf("json print output = %#v", printed)
	}

	runCLI(t, dir, "set", "idle", "--agent", "opencode", "--id", "pane-1")
	statuses = listStatuses(t, dir)
	if len(statuses) != 1 || statuses[0].State != "idle" {
		t.Fatalf("updated status = %#v", statuses)
	}
}

func TestMultipleStatusesAreDeterministic(t *testing.T) {
	dir := t.TempDir()

	runCLI(t, dir, "set", "busy", "--agent", "zeta", "--id", "z")
	runCLI(t, dir, "set", "idle", "--agent", "alpha", "--id", "a")

	statuses := listStatuses(t, dir)
	if len(statuses) != 2 {
		t.Fatalf("got %d statuses, want 2", len(statuses))
	}
	if statuses[0].Agent != "alpha" || statuses[1].Agent != "zeta" {
		t.Fatalf("statuses not sorted by agent: %#v", statuses)
	}

	plain := strings.TrimSpace(runCLI(t, dir, "print", "--format", "plain"))
	if plain != "alpha idle\nzeta busy" {
		t.Fatalf("plain output = %q", plain)
	}
}

func TestMalformedFilesAreIgnoredAndTTLCanGoStale(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	runCLI(t, dir, "set", "busy", "--agent", "opencode", "--id", "short", "--ttl", "1ms")
	time.Sleep(20 * time.Millisecond)

	statuses := listStatuses(t, dir)
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if statuses[0].State != "stale" {
		t.Fatalf("state = %q, want stale", statuses[0].State)
	}
}

func TestClearByAgentAndAll(t *testing.T) {
	dir := t.TempDir()

	runCLI(t, dir, "set", "busy", "--agent", "opencode", "--id", "one")
	runCLI(t, dir, "set", "attention", "--agent", "claude", "--id", "two")
	runCLI(t, dir, "clear", "--agent", "opencode")

	statuses := listStatuses(t, dir)
	if len(statuses) != 1 || statuses[0].Agent != "claude" {
		t.Fatalf("after clear --agent: %#v", statuses)
	}

	runCLI(t, dir, "clear")
	if output := strings.TrimSpace(runCLI(t, dir, "list", "--format", "json")); output != "[]" {
		t.Fatalf("empty list output = %q", output)
	}
	statuses = listStatuses(t, dir)
	if len(statuses) != 0 {
		t.Fatalf("after clear all: %#v", statuses)
	}
}

func TestInvalidStateFails(t *testing.T) {
	dir := t.TempDir()
	cmd := cliCommand(dir, "set", "waiting", "--agent", "opencode")
	if err := cmd.Run(); err == nil {
		t.Fatal("invalid state succeeded")
	}

	cmd = cliCommand(dir, "set", "busy", "--agent", "opencode", "--ttl", "-1s")
	if err := cmd.Run(); err == nil {
		t.Fatal("negative ttl succeeded")
	}
}

func listStatuses(t *testing.T, dir string) []status {
	t.Helper()
	var statuses []status
	decodeJSON(t, runCLI(t, dir, "list", "--format", "json"), &statuses)
	return statuses
}

func runCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := cliCommand(dir, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run .. %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func cliCommand(dir string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{"run", ".."}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Env = testEnv(dir)
	return cmd
}

func testEnv(dir string) []string {
	env := []string{}
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "AGENT_STATUS_DIR=") || strings.HasPrefix(item, "OPEN_SPINNER_DIR=") || strings.HasPrefix(item, "OPEN_SPINNER_ID=") || strings.HasPrefix(item, "AGENT_STATUS_ID=") || strings.HasPrefix(item, "TMUX_PANE=") {
			continue
		}
		env = append(env, item)
	}
	return append(env, "AGENT_STATUS_DIR="+dir)
}

func decodeJSON(t *testing.T, input string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(input), target); err != nil {
		t.Fatalf("invalid json %q: %v", input, err)
	}
}
