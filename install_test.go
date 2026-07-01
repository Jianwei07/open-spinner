package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readJSONFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse %s: %v (content: %s)", path, err, data)
	}
	return root
}

func countManagedGroups(groups []interface{}) int {
	n := 0
	for _, g := range groups {
		if isManagedHookGroup(g) {
			n++
		}
	}
	return n
}

// --- Claude Code ---

func TestInstallClaudeIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := installClaude(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := installClaude(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, filepath.Join(home, ".claude", "settings.json"))
	hooks := root["hooks"].(map[string]interface{})
	for _, event := range []string{"UserPromptSubmit", "Notification", "Stop"} {
		groups, ok := hooks[event].([]interface{})
		if !ok {
			t.Fatalf("event %s missing after install", event)
		}
		if got := countManagedGroups(groups); got != 1 {
			t.Fatalf("event %s: got %d managed groups after two installs, want exactly 1", event, got)
		}
	}
}

func TestInstallClaudePreservesUnrelatedConfig(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{
		"permissions": {"allow": ["Bash(ls:*)"]},
		"hooks": {
			"UserPromptSubmit": [
				{"hooks": [{"type": "command", "command": "echo user-hook"}]}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installClaude(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, path)
	if _, ok := root["permissions"]; !ok {
		t.Fatal("unrelated top-level key 'permissions' was dropped by install")
	}
	groups := root["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{})
	if len(groups) != 2 {
		t.Fatalf("expected user's existing hook group + our managed group (2), got %d", len(groups))
	}
	if countManagedGroups(groups) != 1 {
		t.Fatal("expected exactly one managed group among the two")
	}
}

func TestUninstallClaudeRemovesOnlyManagedGroup(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"hooks": {"UserPromptSubmit": [{"hooks": [{"type": "command", "command": "echo user-hook"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installClaude(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallClaude(home); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, path)
	groups := root["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{})
	if len(groups) != 1 {
		t.Fatalf("expected only the user's original group to remain, got %d", len(groups))
	}
	if countManagedGroups(groups) != 0 {
		t.Fatal("managed group still present after uninstall")
	}
}

func TestUninstallClaudeNoOpWhenNeverInstalled(t *testing.T) {
	home := t.TempDir()
	if err := uninstallClaude(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("uninstall should not create settings.json when there was nothing to remove")
	}
}

// --- Codex ---

func TestInstallCodexIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := installCodex(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := installCodex(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, filepath.Join(home, ".codex", "hooks.json"))
	for _, event := range []string{"UserPromptSubmit", "Notification", "Stop"} {
		arr, ok := root[event].([]interface{})
		if !ok {
			t.Fatalf("event %s missing after install", event)
		}
		managed := 0
		for _, e := range arr {
			if isManagedEntry(e) {
				managed++
			}
		}
		if managed != 1 {
			t.Fatalf("event %s: got %d managed entries after two installs, want exactly 1", event, managed)
		}
	}
}

func TestUninstallCodexRemovesOnlyManagedEntry(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"UserPromptSubmit": [{"command": "echo user"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installCodex(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallCodex(home); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, path)
	arr := root["UserPromptSubmit"].([]interface{})
	if len(arr) != 1 {
		t.Fatalf("expected only the user's entry to remain, got %d", len(arr))
	}
}

// --- OpenCode ---

func TestInstallOpenCodeIsIdempotent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "opencode", "plugin", "open-spinner.js")

	if err := installOpenCode(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := installOpenCode(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("reinstall produced different content; expected deterministic idempotent output")
	}
	if !strings.Contains(string(first), managedFileMarker) {
		t.Fatal("plugin file missing managed marker")
	}
}

func TestInstallOpenCodeRefusesToClobberUnmanagedFile(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "opencode", "plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "open-spinner.js")
	const custom = "// my own custom plugin\n"
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installOpenCode(home, "/bin/open-spinner"); err == nil {
		t.Fatal("expected install to refuse overwriting an unmanaged plugin file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Fatal("unmanaged plugin file content was modified")
	}
}

func TestUninstallOpenCodeRemovesOnlyManagedFile(t *testing.T) {
	home := t.TempDir()
	if err := installOpenCode(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallOpenCode(home); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".config", "opencode", "plugin", "open-spinner.js")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("managed plugin file should be removed after uninstall")
	}
}

func TestUninstallOpenCodeLeavesUnmanagedFileAlone(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "opencode", "plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "open-spinner.js")
	const custom = "// my own custom plugin\n"
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := uninstallOpenCode(home); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Fatal("uninstall modified an unmanaged plugin file")
	}
}

// --- top-level install/uninstall commands (autodetection) ---

func TestInstallCmdAutodetectsFromConfigDirs(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	var out bytes.Buffer
	if err := installCmd(nil, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "claude") || !strings.Contains(got, "codex") {
		t.Fatalf("expected claude and codex to be autodetected and installed, got %q", got)
	}
	if strings.Contains(got, "opencode") {
		t.Fatalf("opencode config dir wasn't present, should not have been installed: %q", got)
	}
}

func TestInstallCmdErrorsWhenNothingDetectedAndNoneSpecified(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	if err := installCmd(nil, &out); err == nil {
		t.Fatal("expected error when no known agent config dirs exist and no agents were named explicitly")
	}
}

func TestUninstallCmdDefaultsToAllKnownAgentsAsNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	if err := uninstallCmd(nil, &out); err != nil {
		t.Fatal(err)
	}
}
