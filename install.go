package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// managedMarkerValue tags every hook entry/config file open-spinner writes,
// so install is idempotent (rerun replaces only our own entries) and
// uninstall never touches a user's other hooks or a plugin file we didn't
// create.
const managedMarkerValue = "open-spinner"
const managedFileMarker = "managed-by: open-spinner"
const pathBlockStart = "# >>> open-spinner >>>"
const pathBlockEnd = "# <<< open-spinner <<<"

var knownAgents = []string{"claude", "codex", "opencode", "qwen", "cursor", "pi", "jcode", "zai", "mimo"}

// shimAgents are hookless agents (no confirmed lifecycle hook/plugin system)
// that get installed via a PATH shim around the `run` wrapper instead of
// hook config. zai and mimo land here deliberately: mimo is a fork of
// OpenCode and may retain its plugin system, but its plugin directory
// convention isn't confirmed anywhere, and a guessed path would silently
// never load (exactly how the Codex "Notification" event bug happened).
// The shim is coarse (whole-process busy) but always correct.
var shimAgents = []string{"pi", "jcode", "zai", "mimo"}

func installCmd(args []string, out io.Writer) error {
	fs := newFlagSet("install")
	if err := fs.Parse(args); err != nil {
		return err
	}

	agents, err := resolveAgentTargets(fs.Args(), true)
	if err != nil {
		return err
	}

	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve open-spinner binary path: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	var done []string
	for _, agent := range agents {
		if err := installAgent(agent, home, bin); err != nil {
			return fmt.Errorf("install %s: %w", agent, err)
		}
		done = append(done, agent)
	}
	fmt.Fprintln(out, "installed: "+strings.Join(done, ", "))
	return nil
}

func uninstallCmd(args []string, out io.Writer) error {
	fs := newFlagSet("uninstall")
	if err := fs.Parse(args); err != nil {
		return err
	}

	agents, err := resolveAgentTargets(fs.Args(), false)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	var done []string
	for _, agent := range agents {
		if err := uninstallAgent(agent, home); err != nil {
			return fmt.Errorf("uninstall %s: %w", agent, err)
		}
		done = append(done, agent)
	}
	fmt.Fprintln(out, "uninstalled: "+strings.Join(done, ", "))
	return nil
}

// resolveAgentTargets returns the explicit agent list if given, otherwise
// auto-detects by config-directory presence (install) or simply tries all
// known agents so uninstall is a safe no-op wherever nothing was
// installed (uninstall).
func resolveAgentTargets(explicit []string, autodetectRequiresConfig bool) ([]string, error) {
	if len(explicit) > 0 {
		for _, a := range explicit {
			if !containsString(knownAgents, a) {
				return nil, fmt.Errorf("unknown agent %q (known: %s)", a, strings.Join(knownAgents, ", "))
			}
		}
		return explicit, nil
	}
	if !autodetectRequiresConfig {
		return knownAgents, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var detected []string
	if dirExists(filepath.Join(home, ".claude")) {
		detected = append(detected, "claude")
	}
	if dirExists(filepath.Join(home, ".codex")) {
		detected = append(detected, "codex")
	}
	if dirExists(filepath.Join(home, ".config", "opencode")) {
		detected = append(detected, "opencode")
	}
	if dirExists(filepath.Join(home, ".qwen")) {
		detected = append(detected, "qwen")
	}
	if dirExists(filepath.Join(home, ".cursor")) {
		detected = append(detected, "cursor")
	}
	for _, agent := range shimAgents {
		if _, err := exec.LookPath(agent); err == nil {
			detected = append(detected, agent)
		}
	}
	if len(detected) == 0 {
		return nil, errors.New("no known agent config directories or hookless agent binaries found (~/.claude, ~/.codex, ~/.config/opencode, ~/.qwen, ~/.cursor, or pi/jcode/zai/mimo on PATH); pass agent names explicitly, e.g. open-spinner install claude")
	}
	return detected, nil
}

func installAgent(agent, home, bin string) error {
	switch agent {
	case "claude":
		return installClaude(home, bin)
	case "codex":
		return installCodex(home, bin)
	case "opencode":
		return installOpenCode(home, bin)
	case "qwen":
		return installQwen(home, bin)
	case "cursor":
		return installCursor(home, bin)
	case "pi", "jcode", "zai", "mimo":
		return installShimAgent(agent, home, bin)
	default:
		return fmt.Errorf("unknown agent %q", agent)
	}
}

func uninstallAgent(agent, home string) error {
	switch agent {
	case "claude":
		return uninstallClaude(home)
	case "codex":
		return uninstallCodex(home)
	case "opencode":
		return uninstallOpenCode(home)
	case "qwen":
		return uninstallQwen(home)
	case "cursor":
		return uninstallCursor(home)
	case "pi", "jcode", "zai", "mimo":
		return uninstallShimAgent(agent, home)
	default:
		return fmt.Errorf("unknown agent %q", agent)
	}
}

// --- Claude Code: ~/.claude/settings.json hooks ---

func installClaude(home, bin string) error {
	path := filepath.Join(home, ".claude", "settings.json")
	root, _, err := loadJSONObjectOrEmpty(path)
	if err != nil {
		return err
	}

	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	for event, verb := range claudeEventVerbs() {
		command := fmt.Sprintf("%s %s --agent claude", bin, verb)
		hooks[event] = upsertManagedHookGroup(hooks[event], command)
	}
	root["hooks"] = hooks
	return saveJSONObject(path, root)
}

func uninstallClaude(home string) error {
	path := filepath.Join(home, ".claude", "settings.json")
	root, existed, err := loadJSONObjectOrEmpty(path)
	if err != nil || !existed {
		return err
	}

	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}

	changed := false
	for event, val := range hooks {
		arr, ok := val.([]interface{})
		if !ok {
			continue
		}
		var kept []interface{}
		for _, group := range arr {
			if isManagedHookGroup(group) {
				changed = true
				continue
			}
			kept = append(kept, group)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if !changed {
		return nil
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = hooks
	}
	return saveJSONObject(path, root)
}

func claudeEventVerbs() map[string]string {
	return map[string]string{
		"UserPromptSubmit": "set busy",
		"Notification":     "set attention",
		"Stop":             "set idle",
	}
}

func upsertManagedHookGroup(existing interface{}, command string) []interface{} {
	var groups []interface{}
	if arr, ok := existing.([]interface{}); ok {
		for _, g := range arr {
			if !isManagedHookGroup(g) {
				groups = append(groups, g)
			}
		}
	}
	groups = append(groups, map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"type":       "command",
				"command":    command,
				"_managedBy": managedMarkerValue,
			},
		},
	})
	return groups
}

func isManagedHookGroup(g interface{}) bool {
	obj, ok := g.(map[string]interface{})
	if !ok {
		return false
	}
	hooksArr, ok := obj["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, h := range hooksArr {
		if isManagedEntry(h) {
			return true
		}
	}
	return false
}

// --- Qwen Code: ~/.qwen/settings.json hooks ---
//
// Qwen Code's hook system is a confirmed Claude Code-compatible clone: same
// nested matcher/hooks group schema, same event names, and stdin JSON
// confirmed to carry session_id (see plan doc for sources). Reuses
// upsertManagedHookGroup/isManagedHookGroup verbatim.

func installQwen(home, bin string) error {
	path := filepath.Join(home, ".qwen", "settings.json")
	root, _, err := loadJSONObjectOrEmpty(path)
	if err != nil {
		return err
	}

	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	for event, verb := range qwenEventVerbs() {
		command := fmt.Sprintf("%s %s --agent qwen", bin, verb)
		hooks[event] = upsertManagedHookGroup(hooks[event], command)
	}
	root["hooks"] = hooks
	return saveJSONObject(path, root)
}

func uninstallQwen(home string) error {
	path := filepath.Join(home, ".qwen", "settings.json")
	root, existed, err := loadJSONObjectOrEmpty(path)
	if err != nil || !existed {
		return err
	}

	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}

	changed := false
	for event, val := range hooks {
		arr, ok := val.([]interface{})
		if !ok {
			continue
		}
		var kept []interface{}
		for _, group := range arr {
			if isManagedHookGroup(group) {
				changed = true
				continue
			}
			kept = append(kept, group)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if !changed {
		return nil
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = hooks
	}
	return saveJSONObject(path, root)
}

func qwenEventVerbs() map[string]string {
	return map[string]string{
		"UserPromptSubmit": "set busy",
		"Notification":     "set attention",
		"Stop":             "set idle",
	}
}

// --- Codex: ~/.codex/hooks.json ---

func installCodex(home, bin string) error {
	path := filepath.Join(home, ".codex", "hooks.json")
	root, _, err := loadJSONObjectOrEmpty(path)
	if err != nil {
		return err
	}

	for event, verb := range codexEventVerbs() {
		command := fmt.Sprintf("%s %s --agent codex", bin, verb)
		root[event] = upsertManagedEntries(root[event], command)
	}
	return saveJSONObject(path, root)
}

func uninstallCodex(home string) error {
	path := filepath.Join(home, ".codex", "hooks.json")
	root, existed, err := loadJSONObjectOrEmpty(path)
	if err != nil || !existed {
		return err
	}

	changed := false
	for event, val := range root {
		arr, ok := val.([]interface{})
		if !ok {
			continue
		}
		var kept []interface{}
		for _, e := range arr {
			if isManagedEntry(e) {
				changed = true
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(root, event)
		} else {
			root[event] = kept
		}
	}
	if !changed {
		return nil
	}
	return saveJSONObject(path, root)
}

func codexEventVerbs() map[string]string {
	return map[string]string{
		"UserPromptSubmit":  "set busy",
		"PermissionRequest": "set attention",
		"Stop":              "set idle",
	}
}

// --- Cursor CLI: ~/.cursor/hooks.json ---
//
// Confirmed via cursor.com/docs/hooks: a flat schema, genuinely different
// from Claude/Qwen's nested matcher/hooks groups — {"version": 1, "hooks":
// {"<event>": [{"command", "type", ...}]}}. No dedicated installer for
// "attention": Cursor has no clean analog (the closest events,
// beforeShellExecution/beforeMCPExecution, are approval-decision hooks
// that likely expect a specific allow/deny JSON on stdout, which
// open-spinner's fire-and-forget `set` does not produce — wiring that
// blind risks silently blocking the user's own shell commands, a worse
// failure than a missing tab glyph). Busy/idle only, deliberately.

func installCursor(home, bin string) error {
	path := filepath.Join(home, ".cursor", "hooks.json")
	root, _, err := loadJSONObjectOrEmpty(path)
	if err != nil {
		return err
	}
	if _, ok := root["version"]; !ok {
		root["version"] = 1
	}

	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	for event, verb := range cursorEventVerbs() {
		command := fmt.Sprintf("%s %s --agent cursor", bin, verb)
		hooks[event] = upsertManagedCursorEntries(hooks[event], command)
	}
	root["hooks"] = hooks
	return saveJSONObject(path, root)
}

func uninstallCursor(home string) error {
	path := filepath.Join(home, ".cursor", "hooks.json")
	root, existed, err := loadJSONObjectOrEmpty(path)
	if err != nil || !existed {
		return err
	}

	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}

	changed := false
	for event, val := range hooks {
		arr, ok := val.([]interface{})
		if !ok {
			continue
		}
		var kept []interface{}
		for _, e := range arr {
			if isManagedEntry(e) {
				changed = true
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if !changed {
		return nil
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = hooks
	}
	return saveJSONObject(path, root)
}

func cursorEventVerbs() map[string]string {
	return map[string]string{
		"beforeSubmitPrompt": "set busy",
		"stop":               "set idle",
	}
}

// upsertManagedCursorEntries mirrors upsertManagedEntries but includes the
// "type": "command" field Cursor's documented schema shows on every entry.
// Kept separate from Codex's helper rather than adding the field there:
// Codex's hook config schema wasn't confirmed to tolerate an extra field
// the same way (its stdin *payload* schemas use deny_unknown_fields, and
// the config-side schema wasn't verified either way) — safer to not risk
// a working integration to save a few lines.
func upsertManagedCursorEntries(existing interface{}, command string) []interface{} {
	var entries []interface{}
	if arr, ok := existing.([]interface{}); ok {
		for _, e := range arr {
			if !isManagedEntry(e) {
				entries = append(entries, e)
			}
		}
	}
	entries = append(entries, map[string]interface{}{
		"type":       "command",
		"command":    command,
		"_managedBy": managedMarkerValue,
	})
	return entries
}

func upsertManagedEntries(existing interface{}, command string) []interface{} {
	var entries []interface{}
	if arr, ok := existing.([]interface{}); ok {
		for _, e := range arr {
			if !isManagedEntry(e) {
				entries = append(entries, e)
			}
		}
	}
	entries = append(entries, map[string]interface{}{
		"command":    command,
		"_managedBy": managedMarkerValue,
	})
	return entries
}

func isManagedEntry(e interface{}) bool {
	m, ok := e.(map[string]interface{})
	if !ok {
		return false
	}
	v, _ := m["_managedBy"].(string)
	return v == managedMarkerValue
}

// --- OpenCode: ~/.config/opencode/plugin/open-spinner.js ---

func installOpenCode(home, bin string) error {
	dir := filepath.Join(home, ".config", "opencode", "plugin")
	path := filepath.Join(dir, "open-spinner.js")

	if data, err := os.ReadFile(path); err == nil {
		if !strings.Contains(string(data), managedFileMarker) {
			return fmt.Errorf("%s already exists and isn't open-spinner-managed; remove it or rename it, then retry", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(openCodePluginSource(bin)), 0o644)
}

func uninstallOpenCode(home string) error {
	path := filepath.Join(home, ".config", "opencode", "plugin", "open-spinner.js")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), managedFileMarker) {
		return nil
	}
	return os.Remove(path)
}

func openCodePluginSource(bin string) string {
	return fmt.Sprintf(`// %s
// Regenerate with: open-spinner install opencode
// Bridges OpenCode's plugin events to the open-spinner status store.
import { execFile } from "node:child_process";

const BIN = %q;

function report(...args) {
  execFile(BIN, args, () => {});
}

// Pull a per-session id out of whatever shape the event carries it in.
// Without this, every OpenCode session on the machine writes the same
// "--agent opencode" status file and stomps on each other's state (a
// finished session can get flipped back to busy by an unrelated one).
// Falls back to "" (open-spinner then keys off the tty instead) if none
// of these match a future OpenCode event shape.
function sessionIdFrom(event) {
  const props = event.properties || {};
  return props.sessionID || props.sessionId || (props.info && props.info.id) || "";
}

export const OpenSpinnerPlugin = async () => {
  return {
    event: async ({ event }) => {
      const id = sessionIdFrom(event);
      const idArgs = id ? ["--id", id] : [];
      switch (event.type) {
        case "session.created":
        case "session.status":
          report("set", "busy", "--agent", "opencode", ...idArgs);
          break;
        case "permission.asked":
          report("set", "attention", "--agent", "opencode", ...idArgs);
          break;
        case "session.idle":
          report("set", "idle", "--agent", "opencode", ...idArgs);
          break;
      }
    },
  };
};
`, managedFileMarker, bin)
}

// --- Hookless agents (pi, jcode): PATH shim wrapping `run` ---

func shimDir(home string) string {
	return filepath.Join(home, ".open-spinner", "shims")
}

// installShimAgent writes a PATH shim for a hookless agent (pi, jcode) that
// wraps the real binary in `open-spinner run`, then ensures the shim
// directory is on PATH for future shells. Shared by both agents since the
// only difference between them is the agent name.
func installShimAgent(agent, home, bin string) error {
	dir := shimDir(home)
	path := filepath.Join(dir, agent)

	if data, err := os.ReadFile(path); err == nil {
		if !strings.Contains(string(data), managedFileMarker) {
			return fmt.Errorf("%s already exists and isn't open-spinner-managed; remove it or rename it, then retry", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(shimScriptSource(agent, bin)), 0o755); err != nil {
		return err
	}
	return ensureShimDirOnPath(home)
}

func uninstallShimAgent(agent, home string) error {
	path := filepath.Join(shimDir(home), agent)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), managedFileMarker) {
		return nil
	}
	// Deliberately does not touch the PATH rc block: a stale PATH entry
	// pointing at an empty/missing shim dir is an inert no-op for every
	// shell, and safely round-tripping edits to a user's rc file (which
	// may have been hand-edited since) is real risk for no real benefit.
	return os.Remove(path)
}

func shimScriptSource(agent, bin string) string {
	return fmt.Sprintf(`#!/bin/sh
# %s
# Regenerate with: open-spinner install %s
#
# Finds the real %q binary on PATH at runtime (skipping this shim's own
# directory, so it doesn't call itself once the shim dir is on PATH) and
# hands it to open-spinner's "run" wrapper, which marks status busy for
# the whole process lifetime. The real binary's path is intentionally
# NOT baked in at install time, since that would go stale on upgrade or
# reinstall of the real CLI.

agent_name=%q
shim_dir=$(cd "$(dirname "$0")" && pwd)

real_bin=""
old_ifs=$IFS
IFS=:
for dir in $PATH; do
    [ -z "$dir" ] && continue
    [ "$dir" = "$shim_dir" ] && continue
    if [ -x "$dir/$agent_name" ]; then
        real_bin="$dir/$agent_name"
        break
    fi
done
IFS=$old_ifs

if [ -z "$real_bin" ]; then
    echo "open-spinner: no real '$agent_name' binary found on PATH (only this shim at $shim_dir)" >&2
    exit 127
fi

exec %q run --agent "$agent_name" -- "$real_bin" "$@"
`, managedFileMarker, agent, agent, agent, bin)
}

// ensureShimDirOnPath appends an idempotent, marker-delimited PATH export
// block to the user's shell rc file, chosen by $SHELL. Shared across both
// pi and jcode installs; installing both writes the block exactly once.
func ensureShimDirOnPath(home string) error {
	rcPath := shellRCPath(home, os.Getenv("SHELL"))

	data, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), pathBlockStart) {
		return nil
	}

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(pathBlock(home))
	return err
}

func shellRCPath(home, shell string) string {
	switch {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zshrc")
	case strings.Contains(shell, "bash"):
		return filepath.Join(home, ".bashrc")
	default:
		return filepath.Join(home, ".profile")
	}
}

func pathBlock(home string) string {
	return fmt.Sprintf("%s\nexport PATH=\"%s:$PATH\"\n%s\n", pathBlockStart, shimDir(home), pathBlockEnd)
}

// --- shared JSON + filesystem helpers ---

func loadJSONObjectOrEmpty(path string) (map[string]interface{}, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]interface{}{}, true, nil
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	return root, true, nil
}

func saveJSONObject(path string, root map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
