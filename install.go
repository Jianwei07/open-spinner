package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// managedMarkerValue tags every hook entry/config file open-spinner writes,
// so install is idempotent (rerun replaces only our own entries) and
// uninstall never touches a user's other hooks or a plugin file we didn't
// create.
const managedMarkerValue = "open-spinner"
const managedFileMarker = "managed-by: open-spinner"

var knownAgents = []string{"claude", "codex", "opencode"}

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
	if len(detected) == 0 {
		return nil, errors.New("no known agent config directories found (~/.claude, ~/.codex, ~/.config/opencode); pass agent names explicitly, e.g. open-spinner install claude")
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
		"UserPromptSubmit": "set busy",
		"Notification":     "set attention",
		"Stop":             "set idle",
	}
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

export const OpenSpinnerPlugin = async () => {
  return {
    event: async ({ event }) => {
      switch (event.type) {
        case "session.created":
        case "session.status":
          report("set", "busy", "--agent", "opencode");
          break;
        case "permission.asked":
          report("set", "attention", "--agent", "opencode");
          break;
        case "session.idle":
          report("set", "idle", "--agent", "opencode");
          break;
      }
    },
  };
};
`, managedFileMarker, bin)
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
