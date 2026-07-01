package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestShimResolvesAndExecsRealBinary proves the PATH shim `open-spinner
// install pi` writes actually finds the real `pi` binary on PATH at
// runtime (skipping its own shim directory), execs it with args/exit
// code/stdio passed through via `run`, and clears status on exit. No PTY
// involved: the shim/run path never touches a tty.
func TestShimResolvesAndExecsRealBinary(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	fakeBinDir := t.TempDir()
	statusDir := t.TempDir()

	fakePi := filepath.Join(fakeBinDir, "pi")
	if err := os.WriteFile(fakePi, []byte("#!/bin/sh\necho fake-pi ran with args: \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	installCmd := exec.Command(bin, "install", "pi")
	installCmd.Env = append(os.Environ(), "HOME="+home, "SHELL=/bin/zsh", "PATH="+fakeBinDir)
	if out, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("install pi failed: %v\n%s", err, out)
	}

	shimPath := filepath.Join(home, ".open-spinner", "shims", "pi")
	data, err := os.ReadFile(shimPath)
	if err != nil {
		t.Fatalf("shim not written: %v", err)
	}
	if !strings.Contains(string(data), "managed-by: open-spinner") {
		t.Fatal("shim script missing managed marker")
	}
	info, err := os.Stat(shimPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("shim script is not executable")
	}

	rcData, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("expected .zshrc to be written: %v", err)
	}
	if !strings.Contains(string(rcData), "open-spinner") {
		t.Fatalf(".zshrc missing PATH block: %s", rcData)
	}

	// Exec the shim directly: it should find the fake pi on PATH (skipping
	// its own shim directory) and pass args through unchanged.
	runCmd := exec.Command(shimPath, "hello", "world")
	runCmd.Env = append(os.Environ(), "HOME="+home, "PATH="+fakeBinDir, "AGENT_STATUS_DIR="+statusDir)
	out, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shim exec failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "fake-pi ran with args: hello world") {
		t.Fatalf("shim did not pass args through to the real binary, got: %s", out)
	}

	// `run` clears status on exit; confirm nothing is left behind.
	listCmd := exec.Command(bin, "list", "--format", "json")
	listCmd.Env = append(os.Environ(), "AGENT_STATUS_DIR="+statusDir)
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list failed: %v\n%s", err, listOut)
	}
	if strings.TrimSpace(string(listOut)) != "[]" {
		t.Fatalf("expected no leftover status after shim exit, got %s", listOut)
	}
}

// TestShimMarksAgentBusyWhileRunning proves the shim's `run` wrapper marks
// status busy for the real binary's whole lifetime, not just at start/end.
func TestShimMarksAgentBusyWhileRunning(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	fakeBinDir := t.TempDir()
	statusDir := t.TempDir()

	fakePi := filepath.Join(fakeBinDir, "pi")
	if err := os.WriteFile(fakePi, []byte("#!/bin/sh\nsleep 0.5\necho done\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	installCmd := exec.Command(bin, "install", "pi")
	installCmd.Env = append(os.Environ(), "HOME="+home, "SHELL=/bin/zsh", "PATH="+fakeBinDir)
	if out, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("install pi failed: %v\n%s", err, out)
	}
	shimPath := filepath.Join(home, ".open-spinner", "shims", "pi")

	runCmd := exec.Command(shimPath)
	runCmd.Env = append(os.Environ(), "HOME="+home, "PATH="+fakeBinDir, "AGENT_STATUS_DIR="+statusDir)
	if err := runCmd.Start(); err != nil {
		t.Fatalf("failed to start shim: %v", err)
	}

	sawBusy := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listCmd := exec.Command(bin, "list", "--format", "json")
		listCmd.Env = append(os.Environ(), "AGENT_STATUS_DIR="+statusDir)
		out, err := listCmd.CombinedOutput()
		if err == nil && strings.Contains(string(out), `"agent": "pi"`) && strings.Contains(string(out), `"state": "busy"`) {
			sawBusy = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := runCmd.Wait(); err != nil {
		t.Fatalf("shim process failed: %v", err)
	}
	if !sawBusy {
		t.Fatal("expected agent pi to be marked busy while the shim's child process was running")
	}
}

// TestShimErrorsWhenNoRealBinaryOnPath proves the shim fails clearly
// (nonzero exit, stderr message) instead of silently doing nothing when no
// underlying real binary can be found.
func TestShimErrorsWhenNoRealBinaryOnPath(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	emptyBinDir := t.TempDir()

	installCmd := exec.Command(bin, "install", "pi")
	installCmd.Env = append(os.Environ(), "HOME="+home, "SHELL=/bin/zsh", "PATH="+filepath.Join(home, "fake-pi-src"))
	// Seed a fake pi just so install can succeed without erroring on
	// autodetection specifics; install itself doesn't require a real
	// binary to exist, only writes the shim.
	if out, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("install pi failed: %v\n%s", err, out)
	}
	shimPath := filepath.Join(home, ".open-spinner", "shims", "pi")

	runCmd := exec.Command(shimPath)
	runCmd.Env = append(os.Environ(), "HOME="+home, "PATH="+emptyBinDir)
	out, err := runCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected shim to fail when no real pi binary is on PATH, got: %s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 127 {
		t.Fatalf("expected exit code 127, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(string(out), "no real 'pi' binary found") {
		t.Fatalf("expected clear error message, got: %s", out)
	}
}
