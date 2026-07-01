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

// --- Qwen Code ---

func TestInstallQwenIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := installQwen(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := installQwen(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, filepath.Join(home, ".qwen", "settings.json"))
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

func TestInstallQwenPreservesUnrelatedConfig(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".qwen", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{
		"model": "qwen3-coder-plus",
		"hooks": {
			"UserPromptSubmit": [
				{"hooks": [{"type": "command", "command": "echo user-hook"}]}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installQwen(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, path)
	if _, ok := root["model"]; !ok {
		t.Fatal("unrelated top-level key 'model' was dropped by install")
	}
	groups := root["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{})
	if len(groups) != 2 {
		t.Fatalf("expected user's existing hook group + our managed group (2), got %d", len(groups))
	}
}

func TestUninstallQwenRemovesOnlyManagedGroup(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".qwen", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"hooks": {"UserPromptSubmit": [{"hooks": [{"type": "command", "command": "echo user-hook"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installQwen(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallQwen(home); err != nil {
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
	for _, event := range []string{"UserPromptSubmit", "PermissionRequest", "Stop"} {
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

// --- Cursor CLI ---

func TestInstallCursorIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := installCursor(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := installCursor(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, filepath.Join(home, ".cursor", "hooks.json"))
	if v, _ := root["version"].(float64); v != 1 {
		t.Fatalf("expected version 1, got %v", root["version"])
	}
	hooks := root["hooks"].(map[string]interface{})
	for _, event := range []string{"beforeSubmitPrompt", "stop"} {
		arr, ok := hooks[event].([]interface{})
		if !ok {
			t.Fatalf("event %s missing after install", event)
		}
		managed := 0
		for _, e := range arr {
			if isManagedEntry(e) {
				managed++
			}
			m := e.(map[string]interface{})
			if m["type"] != "command" {
				t.Fatalf("event %s entry missing type:command: %v", event, m)
			}
		}
		if managed != 1 {
			t.Fatalf("event %s: got %d managed entries after two installs, want exactly 1", event, managed)
		}
	}
	// No attention mapping for Cursor — deliberate, documented gap.
	if _, ok := hooks["Notification"]; ok {
		t.Fatal("cursor installer should not write an attention/Notification hook")
	}
}

func TestInstallCursorPreservesUnrelatedConfigAndVersion(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"version": 1, "hooks": {"beforeSubmitPrompt": [{"type": "command", "command": "echo user"}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installCursor(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, path)
	arr := root["hooks"].(map[string]interface{})["beforeSubmitPrompt"].([]interface{})
	if len(arr) != 2 {
		t.Fatalf("expected user's entry + our managed entry (2), got %d", len(arr))
	}
}

func TestUninstallCursorRemovesOnlyManagedEntry(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"version": 1, "hooks": {"beforeSubmitPrompt": [{"type": "command", "command": "echo user"}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installCursor(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallCursor(home); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, path)
	arr := root["hooks"].(map[string]interface{})["beforeSubmitPrompt"].([]interface{})
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

// Without a per-session id, every OpenCode session writes the same
// "--agent opencode" status file and stomps on each other's state — the
// same collision bug that made the native-tab spinner get stuck across
// multiple Claude/Codex sessions. Guard that the generated plugin always
// threads a session id through to each report(...) call.
func TestInstallOpenCodePluginPassesSessionID(t *testing.T) {
	home := t.TempDir()
	if err := installOpenCode(home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "plugin", "open-spinner.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, "sessionIdFrom") {
		t.Fatal("plugin should derive a session id from the event payload")
	}
	for _, call := range []string{`"busy", "--agent", "opencode", ...idArgs`, `"attention", "--agent", "opencode", ...idArgs`, `"idle", "--agent", "opencode", ...idArgs`} {
		if !strings.Contains(src, call) {
			t.Fatalf("expected report(...) call to thread idArgs through: %s", call)
		}
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

// --- Hookless agents (pi, jcode) ---

func TestInstallShimAgentIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHELL", "/bin/zsh")
	path := filepath.Join(home, ".open-spinner", "shims", "pi")

	if err := installShimAgent("pi", home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := installShimAgent("pi", home, "/bin/open-spinner"); err != nil {
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
		t.Fatal("shim script missing managed marker")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("shim script is not executable")
	}
}

func TestInstallShimAgentRefusesToClobberUnmanagedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHELL", "/bin/zsh")
	dir := filepath.Join(home, ".open-spinner", "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pi")
	const custom = "#!/bin/sh\necho my own pi wrapper\n"
	if err := os.WriteFile(path, []byte(custom), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installShimAgent("pi", home, "/bin/open-spinner"); err == nil {
		t.Fatal("expected install to refuse overwriting an unmanaged shim file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Fatal("unmanaged shim file content was modified")
	}
}

func TestUninstallShimAgentRemovesOnlyManagedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHELL", "/bin/zsh")
	if err := installShimAgent("pi", home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallShimAgent("pi", home); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".open-spinner", "shims", "pi")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("managed shim file should be removed after uninstall")
	}
}

func TestUninstallShimAgentLeavesUnmanagedFileAlone(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".open-spinner", "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pi")
	const custom = "#!/bin/sh\necho my own pi wrapper\n"
	if err := os.WriteFile(path, []byte(custom), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := uninstallShimAgent("pi", home); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Fatal("uninstall modified an unmanaged shim file")
	}
}

func TestEnsureShimDirOnPathIsIdempotentAcrossBothAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHELL", "/bin/zsh")

	if err := installShimAgent("pi", home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}
	if err := installShimAgent("jcode", home, "/bin/open-spinner"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), pathBlockStart); got != 1 {
		t.Fatalf("expected PATH block written exactly once across both installs, got %d", got)
	}
}

func TestShellRCPathSelectsFileByShellEnv(t *testing.T) {
	home := t.TempDir()
	cases := map[string]string{
		"/bin/zsh":      ".zshrc",
		"/usr/bin/bash": ".bashrc",
		"/usr/bin/fish": ".profile",
		"":              ".profile",
	}
	for shell, want := range cases {
		got := shellRCPath(home, shell)
		if filepath.Base(got) != want {
			t.Errorf("shellRCPath(%q) = %q, want basename %q", shell, got, want)
		}
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

func TestInstallCmdAutodetectsQwenAndCursorFromConfigDirs(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".qwen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	var out bytes.Buffer
	if err := installCmd(nil, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "qwen") || !strings.Contains(got, "cursor") {
		t.Fatalf("expected qwen and cursor to be autodetected and installed, got %q", got)
	}
}

func TestInstallCmdAutodetectsZaiAndMimoFromPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	fakeBinDir := t.TempDir()
	for _, agent := range []string{"zai", "mimo"} {
		fakeBin := filepath.Join(fakeBinDir, agent)
		if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho fake\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", fakeBinDir)

	var out bytes.Buffer
	if err := installCmd(nil, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "zai") || !strings.Contains(got, "mimo") {
		t.Fatalf("expected zai and mimo to be autodetected via PATH and installed, got %q", got)
	}
}

func TestInstallCmdAutodetectsHooklessAgentFromPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	fakeBinDir := t.TempDir()
	fakePi := filepath.Join(fakeBinDir, "pi")
	if err := os.WriteFile(fakePi, []byte("#!/bin/sh\necho fake-pi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBinDir)

	var out bytes.Buffer
	if err := installCmd(nil, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "pi") {
		t.Fatalf("expected pi to be autodetected via PATH and installed, got %q", got)
	}
	if strings.Contains(got, "jcode") || strings.Contains(got, "claude") || strings.Contains(got, "codex") || strings.Contains(got, "opencode") {
		t.Fatalf("only pi should have been detected, got %q", got)
	}
}

func TestInstallCmdErrorsWhenNothingDetectedAndNoneSpecified(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Empty PATH so pi/jcode autodetection can't accidentally find a real
	// binary installed on the machine running this test.
	t.Setenv("PATH", t.TempDir())
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
