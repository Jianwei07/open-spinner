package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setDoctorEnv(t *testing.T) (home, statusDir string) {
	t.Helper()
	home = t.TempDir()
	statusDir = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENT_STATUS_DIR", statusDir)
	t.Setenv("OPEN_SPINNER_TTY", "/dev/ttys123")
	return home, statusDir
}

func TestDoctorWarnsForWezTermFormatterAndLegacyTTY(t *testing.T) {
	home, statusDir := setDoctorEnv(t)
	weztermDir := filepath.Join(home, ".config", "wezterm")
	if err := os.MkdirAll(weztermDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(weztermDir, "wezterm.lua"), []byte(`wezterm.on("format-tab-title", function() end)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeStatus(statusDir, Status{
		V: 1, ID: "session-1", Agent: "opencode", State: "busy", TTY: "/dev/tty",
		UpdatedAt: time.Now().UTC(), TTLMS: time.Minute.Milliseconds(),
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := doctorCmd(nil, &out); err != nil {
		t.Fatalf("doctorCmd returned unexpected error: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{"WARN wezterm", "format-tab-title", "WARN status", "legacy /dev/tty"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorWarnsForHomeWezTermFormatter(t *testing.T) {
	home, _ := setDoctorEnv(t)
	if err := os.WriteFile(filepath.Join(home, ".wezterm.lua"), []byte(`wezterm.on("format-tab-title", function() end)`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := doctorCmd(nil, &out); err != nil {
		t.Fatalf("doctorCmd returned unexpected error: %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, "WARN wezterm") || !strings.Contains(got, ".wezterm.lua") {
		t.Fatalf("doctor output did not report ~/.wezterm.lua formatter:\n%s", got)
	}
}

func TestDoctorChecksAttentionRendererLock(t *testing.T) {
	_, statusDir := setDoctorEnv(t)
	if err := writeStatus(statusDir, Status{
		V: 1, ID: "needs-user", Agent: "opencode", State: "attention", TTY: "/dev/ttys123",
		UpdatedAt: time.Now().UTC(), TTLMS: time.Minute.Milliseconds(),
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := doctorCmd(nil, &out); err != nil {
		t.Fatalf("doctorCmd returned unexpected error: %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, "WARN renderer") || !strings.Contains(got, "active but no renderer lock") {
		t.Fatalf("doctor output did not report missing attention renderer:\n%s", got)
	}
}

func TestDoctorWarnsForInvalidAndStaleStatuses(t *testing.T) {
	_, statusDir := setDoctorEnv(t)
	if err := os.WriteFile(filepath.Join(statusDir, "bad.json"), []byte(`not-json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeStatus(statusDir, Status{
		V: 1, ID: "old", Agent: "codex", State: "busy", TTY: "/dev/ttys123",
		UpdatedAt: time.Now().Add(-time.Hour).UTC(), TTLMS: time.Millisecond.Milliseconds(),
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := doctorCmd(nil, &out); err != nil {
		t.Fatalf("doctorCmd returned unexpected error: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{"invalid JSON", "is stale"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorFailsMissingManagedHookBinary(t *testing.T) {
	home, _ := setDoctorEnv(t)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"UserPromptSubmit":[{"_managedBy":"open-spinner","command":"/missing/open-spinner set busy --agent codex"}]}`
	if err := os.WriteFile(filepath.Join(codexDir, "hooks.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := doctorCmd(nil, &out)
	if err == nil {
		t.Fatalf("doctorCmd returned nil, want failure\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "FAIL codex UserPromptSubmit") || !strings.Contains(got, "/missing/open-spinner") {
		t.Fatalf("doctor output did not report missing hook binary:\n%s", got)
	}
}

func TestDoctorWarnsForLegacyUnmanagedHooks(t *testing.T) {
	home, _ := setDoctorEnv(t)
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"` + bin + ` set busy --agent claude"}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := doctorCmd(nil, &out); err != nil {
		t.Fatalf("doctorCmd returned unexpected error: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "legacy unmanaged open-spinner hooks") || !strings.Contains(got, "OK   claude UserPromptSubmit") {
		t.Fatalf("doctor output did not report legacy hook correctly:\n%s", got)
	}
}

func TestDoctorWarnsForLegacyHooksEvenWhenManagedExists(t *testing.T) {
	home, _ := setDoctorEnv(t)
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"UserPromptSubmit":[` +
		`{"_managedBy":"open-spinner","command":"` + bin + ` set busy --agent codex"},` +
		`{"command":"` + bin + ` set busy --agent codex"}` +
		`]}`
	if err := os.WriteFile(filepath.Join(codexDir, "hooks.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := doctorCmd(nil, &out); err != nil {
		t.Fatalf("doctorCmd returned unexpected error: %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, "legacy unmanaged open-spinner hooks") || !strings.Contains(got, "OK   codex UserPromptSubmit") {
		t.Fatalf("doctor output did not report managed+legacy hooks:\n%s", got)
	}
}

func TestDoctorWarnsWhenHooklessAgentShimIsMissing(t *testing.T) {
	setDoctorEnv(t)
	fakeBin := t.TempDir()
	pi := filepath.Join(fakeBin, "pi")
	if err := os.WriteFile(pi, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)

	var out bytes.Buffer
	if err := doctorCmd(nil, &out); err != nil {
		t.Fatalf("doctorCmd returned unexpected error: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "WARN pi shim") || !strings.Contains(got, "run open-spinner install pi") {
		t.Fatalf("doctor output did not report missing hookless shim:\n%s", got)
	}
}

func TestHasWezTermFormatTabTitleIgnoresComments(t *testing.T) {
	if hasWezTermFormatTabTitle(`-- wezterm.on("format-tab-title", disabled)`) {
		t.Fatal("commented format-tab-title should not trigger warning")
	}
	if !hasWezTermFormatTabTitle(`wezterm.on("format-tab-title", function() end)`) {
		t.Fatal("active format-tab-title should trigger warning")
	}
}

func TestDoctorParsesInstalledBinaryPaths(t *testing.T) {
	if got := doctorOpenCodePluginBin(openCodePluginSource("/tmp/open spinner")); got != "/tmp/open spinner" {
		t.Fatalf("doctorOpenCodePluginBin() = %q", got)
	}
	if got := doctorShimBin(shimScriptSource("pi", "/tmp/open spinner")); got != "/tmp/open spinner" {
		t.Fatalf("doctorShimBin() = %q", got)
	}
}
