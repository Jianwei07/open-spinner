package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type doctorReporter struct {
	out    io.Writer
	failed bool
}

func doctorCmd(args []string, out io.Writer) error {
	fs := newFlagSet("doctor")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: open-spinner doctor")
	}

	reporter := &doctorReporter{out: out}
	home, err := os.UserHomeDir()
	if err != nil {
		reporter.fail("home", err.Error())
	} else {
		doctorCheckIntegrations(home, reporter)
		doctorCheckWezTerm(home, reporter)
	}
	doctorCheckBinary(reporter)
	doctorCheckTTY(reporter)
	doctorCheckStatuses(reporter)

	if reporter.failed {
		return errors.New("doctor found failures")
	}
	return nil
}

func (r *doctorReporter) ok(subject, detail string) {
	fmt.Fprintf(r.out, "OK   %s: %s\n", subject, detail)
}

func (r *doctorReporter) warn(subject, detail string) {
	fmt.Fprintf(r.out, "WARN %s: %s\n", subject, detail)
}

func (r *doctorReporter) fail(subject, detail string) {
	r.failed = true
	fmt.Fprintf(r.out, "FAIL %s: %s\n", subject, detail)
}

func doctorCheckBinary(r *doctorReporter) {
	exe, err := os.Executable()
	if err != nil {
		r.fail("binary", "cannot resolve current executable: "+err.Error())
		return
	}
	if err := executableExists(exe); err != nil {
		r.fail("binary", exe+": "+err.Error())
		return
	}
	r.ok("binary", exe+" exists")
}

func doctorCheckTTY(r *doctorReporter) {
	tty := resolveTTY()
	if tty == "" {
		r.fail("tty", "no concrete tty found; run from a real terminal or set OPEN_SPINNER_TTY=$(tty)")
		return
	}
	r.ok("tty", tty)
}

func doctorCheckIntegrations(home string, r *doctorReporter) {
	doctorCheckNestedHooks(r, "claude", filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "settings.json"), claudeEventVerbs())
	doctorCheckFlatHooks(r, "codex", filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "hooks.json"), codexEventVerbs(), false)
	doctorCheckOpenCode(home, r)
	doctorCheckNestedHooks(r, "qwen", filepath.Join(home, ".qwen"), filepath.Join(home, ".qwen", "settings.json"), qwenEventVerbs())
	doctorCheckFlatHooks(r, "cursor", filepath.Join(home, ".cursor"), filepath.Join(home, ".cursor", "hooks.json"), cursorEventVerbs(), true)
	for _, agent := range shimAgents {
		doctorCheckShim(home, r, agent)
	}
}

func doctorCheckNestedHooks(r *doctorReporter, agent, dir, path string, events map[string]string) {
	if !dirExists(dir) {
		return
	}
	root, existed, err := loadJSONObjectOrEmpty(path)
	if err != nil {
		r.fail(agent+" hooks", err.Error())
		return
	}
	if !existed {
		r.warn(agent+" hooks", "config dir exists but hook file is missing; run open-spinner install "+agent)
		return
	}
	hooks, _ := root["hooks"].(map[string]interface{})
	for event := range events {
		commands := doctorManagedHookGroupCommands(hooks[event])
		legacy := doctorLegacyHookGroupCommands(hooks[event])
		if len(legacy) > 0 {
			r.warn(agent+" hooks", event+" has legacy unmanaged open-spinner hooks; remove duplicate hooks manually because install preserves user entries")
			for _, command := range legacy {
				doctorCheckCommandBinary(r, agent+" "+event, command)
			}
		}
		if len(commands) == 0 {
			if len(legacy) > 0 {
				continue
			}
			r.warn(agent+" hooks", event+" is not installed; run open-spinner install "+agent)
			continue
		}
		for _, command := range commands {
			doctorCheckCommandBinary(r, agent+" "+event, command)
		}
	}
}

func doctorCheckFlatHooks(r *doctorReporter, agent, dir, path string, events map[string]string, nestedUnderHooks bool) {
	if !dirExists(dir) {
		return
	}
	root, existed, err := loadJSONObjectOrEmpty(path)
	if err != nil {
		r.fail(agent+" hooks", err.Error())
		return
	}
	if !existed {
		r.warn(agent+" hooks", "config dir exists but hook file is missing; run open-spinner install "+agent)
		return
	}
	container := root
	if nestedUnderHooks {
		container, _ = root["hooks"].(map[string]interface{})
	}
	for event := range events {
		commands := doctorManagedEntryCommands(container[event])
		legacy := doctorLegacyEntryCommands(container[event])
		if len(legacy) > 0 {
			r.warn(agent+" hooks", event+" has legacy unmanaged open-spinner hooks; remove duplicate hooks manually because install preserves user entries")
			for _, command := range legacy {
				doctorCheckCommandBinary(r, agent+" "+event, command)
			}
		}
		if len(commands) == 0 {
			if len(legacy) > 0 {
				continue
			}
			r.warn(agent+" hooks", event+" is not installed; run open-spinner install "+agent)
			continue
		}
		for _, command := range commands {
			doctorCheckCommandBinary(r, agent+" "+event, command)
		}
	}
}

func doctorCheckOpenCode(home string, r *doctorReporter) {
	dir := filepath.Join(home, ".config", "opencode")
	if !dirExists(dir) {
		return
	}
	path := filepath.Join(dir, "plugin", "open-spinner.js")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		r.warn("opencode plugin", "not installed; run open-spinner install opencode")
		return
	}
	if err != nil {
		r.fail("opencode plugin", err.Error())
		return
	}
	text := string(data)
	if !strings.Contains(text, managedFileMarker) {
		r.fail("opencode plugin", path+" exists but is not open-spinner-managed")
		return
	}
	bin := doctorOpenCodePluginBin(text)
	if bin == "" {
		r.fail("opencode plugin", "cannot find BIN path in "+path)
		return
	}
	doctorCheckBinaryPath(r, "opencode plugin", bin)
}

func doctorCheckShim(home string, r *doctorReporter, agent string) {
	path := filepath.Join(shimDir(home), agent)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if real := doctorRealAgentOnPath(agent, shimDir(home)); real != "" {
			r.warn(agent+" shim", "real "+agent+" found at "+real+" but managed shim is not installed; run open-spinner install "+agent)
		}
		return
	}
	if err != nil {
		r.fail(agent+" shim", err.Error())
		return
	}
	if info, err := os.Stat(path); err != nil {
		r.fail(agent+" shim", err.Error())
		return
	} else if info.Mode()&0o111 == 0 {
		r.fail(agent+" shim", path+" is not executable")
		return
	}
	text := string(data)
	if !strings.Contains(text, managedFileMarker) {
		r.fail(agent+" shim", path+" exists but is not open-spinner-managed")
		return
	}
	bin := doctorShimBin(text)
	if bin == "" {
		r.fail(agent+" shim", "cannot find open-spinner exec path in "+path)
		return
	}
	doctorCheckBinaryPath(r, agent+" shim", bin)
	if !pathListContains(os.Getenv("PATH"), shimDir(home)) {
		r.warn(agent+" shim", shimDir(home)+" is not on current PATH; open a new shell or source your rc file")
	}
	if real := doctorRealAgentOnPath(agent, shimDir(home)); real == "" {
		r.warn(agent+" shim", "no real "+agent+" binary found on PATH after skipping the shim dir")
	}
}

func doctorCheckWezTerm(home string, r *doctorReporter) {
	files := []string{filepath.Join(home, ".wezterm.lua")}
	configFiles, _ := filepath.Glob(filepath.Join(home, ".config", "wezterm", "*.lua"))
	files = append(files, configFiles...)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if hasWezTermFormatTabTitle(string(data)) {
			r.warn("wezterm", "custom format-tab-title in "+path+" may override OSC titles; preserve pane.title/tab.active_pane.title")
			return
		}
	}
}

func hasWezTermFormatTabTitle(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--") {
			continue
		}
		if strings.Contains(line, "wezterm.on") && strings.Contains(line, "format-tab-title") {
			return true
		}
	}
	return false
}

func doctorCheckStatuses(r *doctorReporter) {
	dir := statusDir()
	doctorCheckStatusFiles(dir, r)
	statuses, err := readStatuses(dir, time.Now().UTC())
	if err != nil {
		r.fail("status", err.Error())
		return
	}
	if len(statuses) == 0 {
		r.ok("status", "no active status files in "+dir)
		return
	}
	for _, status := range statuses {
		label := status.Agent + "/" + status.ID
		if status.TTY == "/dev/tty" {
			r.warn("status", label+" uses legacy /dev/tty; restart the agent after rebuilding/reinstalling open-spinner")
		}
		if status.State == "stale" {
			r.warn("status", label+" is stale; restart the agent or run open-spinner clear --id "+status.ID)
			continue
		}
		if status.State != "busy" && status.State != "attention" {
			continue
		}
		if concreteTTY(status.TTY) == "" {
			r.warn("renderer", label+" is active but has no concrete tty")
			continue
		}
		lockPath := filepath.Join(dir, "render-locks", safeName(status.TTY)+".lock")
		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			r.warn("renderer", label+" is active but no renderer lock exists")
			continue
		}
		if lockHolderAlive(lockPath) {
			r.ok("renderer", label+" renderer is running")
		} else {
			r.warn("renderer", label+" has a stale renderer lock")
		}
	}
}

func doctorCheckStatusFiles(dir string, r *doctorReporter) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		r.fail("status", err.Error())
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			r.warn("status", path+" cannot be read: "+err.Error())
			continue
		}
		var status Status
		if err := json.Unmarshal(data, &status); err != nil {
			r.warn("status", path+" is invalid JSON; remove it or run open-spinner clear")
			continue
		}
		if status.ID == "" || status.Agent == "" || !validState(status.State) {
			r.warn("status", path+" is not a valid open-spinner status file")
		}
	}
}

func doctorManagedHookGroupCommands(existing interface{}) []string {
	var commands []string
	groups, _ := existing.([]interface{})
	for _, group := range groups {
		obj, _ := group.(map[string]interface{})
		hooks, _ := obj["hooks"].([]interface{})
		commands = append(commands, doctorManagedEntryCommands(hooks)...)
	}
	return uniqueStrings(commands)
}

func doctorLegacyHookGroupCommands(existing interface{}) []string {
	var commands []string
	groups, _ := existing.([]interface{})
	for _, group := range groups {
		obj, _ := group.(map[string]interface{})
		hooks, _ := obj["hooks"].([]interface{})
		commands = append(commands, doctorLegacyEntryCommands(hooks)...)
	}
	return uniqueStrings(commands)
}

func doctorManagedEntryCommands(existing interface{}) []string {
	var commands []string
	entries, _ := existing.([]interface{})
	for _, entry := range entries {
		if !isManagedEntry(entry) {
			continue
		}
		obj, _ := entry.(map[string]interface{})
		if command, _ := obj["command"].(string); command != "" {
			commands = append(commands, command)
		}
	}
	return uniqueStrings(commands)
}

func doctorLegacyEntryCommands(existing interface{}) []string {
	var commands []string
	entries, _ := existing.([]interface{})
	for _, entry := range entries {
		if isManagedEntry(entry) {
			continue
		}
		obj, _ := entry.(map[string]interface{})
		command, _ := obj["command"].(string)
		if strings.Contains(command, "open-spinner") {
			commands = append(commands, command)
		}
	}
	return uniqueStrings(commands)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func pathListContains(pathList, target string) bool {
	for _, dir := range filepath.SplitList(pathList) {
		if dir == target {
			return true
		}
	}
	return false
}

func doctorRealAgentOnPath(agent, shimDir string) string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || dir == shimDir {
			continue
		}
		path := filepath.Join(dir, agent)
		if executableExists(path) == nil {
			return path
		}
	}
	return ""
}

func doctorCheckCommandBinary(r *doctorReporter, subject, command string) {
	bin := doctorCommandBinary(command)
	if bin == "" {
		r.fail(subject, "managed command is malformed: "+command)
		return
	}
	doctorCheckBinaryPath(r, subject, bin)
}

func doctorCheckBinaryPath(r *doctorReporter, subject, bin string) {
	if err := executableExists(bin); err != nil {
		r.fail(subject, "points to missing binary "+bin+": "+err.Error())
		return
	}
	r.ok(subject, "uses "+bin)
}

func doctorCommandBinary(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], "\"")
}

func executableExists(bin string) error {
	if strings.Contains(bin, string(os.PathSeparator)) {
		info, err := os.Stat(bin)
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return errors.New("not executable")
		}
		return nil
	}
	_, err := exec.LookPath(bin)
	return err
}

func doctorOpenCodePluginBin(src string) string {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "const BIN = ") || !strings.HasSuffix(line, ";") {
			continue
		}
		quoted := strings.TrimSuffix(strings.TrimPrefix(line, "const BIN = "), ";")
		bin, err := strconv.Unquote(quoted)
		if err == nil {
			return bin
		}
	}
	return ""
}

func doctorShimBin(src string) string {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "exec ") {
			continue
		}
		if bin := firstShellToken(strings.TrimSpace(strings.TrimPrefix(line, "exec "))); bin != "" {
			return bin
		}
	}
	return ""
}

func firstShellToken(value string) string {
	if strings.HasPrefix(value, "\"") {
		for i := 1; i < len(value); i++ {
			if value[i] == '\\' {
				i++
				continue
			}
			if value[i] == '"' {
				bin, err := strconv.Unquote(value[:i+1])
				if err == nil {
					return bin
				}
				return ""
			}
		}
		return ""
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], "\"")
}
