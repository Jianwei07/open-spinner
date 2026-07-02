package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const version = "0.1.0-dev"

type Status struct {
	V         int       `json:"v"`
	ID        string    `json:"id"`
	Agent     string    `json:"agent"`
	State     string    `json:"state"`
	Text      string    `json:"text,omitempty"`
	CWD       string    `json:"cwd,omitempty"`
	PID       int       `json:"pid,omitempty"`
	TTY       string    `json:"tty,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	TTLMS     int64     `json:"ttl_ms"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: open-spinner <set|clear|list|print|doctor|version>")
	}

	switch args[0] {
	case "--version", "version":
		fmt.Fprintln(out, version)
		return nil
	case "--doctor", "doctor":
		return doctorCmd(args[1:], out)
	case "set":
		return setCmd(args[1:])
	case "clear":
		return clearCmd(args[1:])
	case "list":
		return listCmd(args[1:], out)
	case "print":
		return printCmd(args[1:], out)
	case "render":
		return renderCmd(args[1:])
	case "run":
		return runWrapperCmd(args[1:])
	case "install":
		return installCmd(args[1:], out)
	case "uninstall":
		return uninstallCmd(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func setCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("set requires a state")
	}

	state := args[0]
	if !validState(state) {
		return fmt.Errorf("invalid state %q", state)
	}

	fs := newFlagSet("set")
	agent := fs.String("agent", "", "agent name")
	text := fs.String("text", "", "status text")
	id := fs.String("id", "", "status id")
	ttl := fs.Duration("ttl", 5*time.Minute, "status ttl")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *agent == "" {
		return errors.New("set requires --agent")
	}
	if *ttl < 0 {
		return errors.New("ttl must be non-negative")
	}

	cwd, _ := os.Getwd()
	status := Status{
		V:         1,
		ID:        resolveID(*id, *agent),
		Agent:     *agent,
		State:     state,
		Text:      *text,
		CWD:       cwd,
		PID:       os.Getpid(),
		TTY:       resolveTTY(),
		UpdatedAt: time.Now().UTC(),
		TTLMS:     ttl.Milliseconds(),
	}

	if err := writeStatus(statusDir(), status); err != nil {
		return err
	}

	if state == "busy" {
		maybeSpawnRenderer(status)
	}
	return nil
}

func clearCmd(args []string) error {
	fs := newFlagSet("clear")
	id := fs.String("id", "", "status id")
	agent := fs.String("agent", "", "agent name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := statusDir()
	if *id != "" {
		return removeStatus(dir, *id)
	}
	if *agent != "" {
		statuses, err := readStatuses(dir, time.Now().UTC())
		if err != nil {
			return err
		}
		for _, status := range statuses {
			if status.Agent == *agent {
				if err := removeStatus(dir, status.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if id := envID(); id != "" {
		return removeStatus(dir, id)
	}
	return clearAll(dir)
}

func listCmd(args []string, out io.Writer) error {
	fs := newFlagSet("list")
	format := fs.String("format", "json", "output format")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "json" {
		return errors.New("list supports only --format json")
	}

	statuses, err := readStatuses(statusDir(), time.Now().UTC())
	if err != nil {
		return err
	}
	return writeJSON(out, statuses)
}

func printCmd(args []string, out io.Writer) error {
	fs := newFlagSet("print")
	format := fs.String("format", "plain", "output format")
	if err := fs.Parse(args); err != nil {
		return err
	}

	statuses, err := readStatuses(statusDir(), time.Now().UTC())
	if err != nil {
		return err
	}

	switch *format {
	case "plain":
		fmt.Fprintln(out, renderPlain(statuses))
	case "tmux":
		fmt.Fprintln(out, renderTmux(statuses))
	case "json":
		return writeJSON(out, statuses)
	default:
		return fmt.Errorf("unsupported print format %q", *format)
	}
	return nil
}

func writeStatus(dir string, status Status) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	target := statusPath(dir, status.ID)
	if err := os.Rename(tmpName, target); err != nil {
		if removeErr := os.Remove(target); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		return os.Rename(tmpName, target)
	}
	return nil
}

func readStatuses(dir string, now time.Time) ([]Status, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var statuses []Status
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var status Status
		if err := json.Unmarshal(data, &status); err != nil {
			continue
		}
		if status.ID == "" || status.Agent == "" || !validState(status.State) {
			continue
		}
		statuses = append(statuses, withDerivedState(status, now))
	}

	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Agent == statuses[j].Agent {
			return statuses[i].ID < statuses[j].ID
		}
		return statuses[i].Agent < statuses[j].Agent
	})
	return statuses, nil
}

func removeStatus(dir, id string) error {
	err := os.Remove(statusPath(dir, id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func clearAll(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func withDerivedState(status Status, now time.Time) Status {
	if status.TTLMS > 0 && now.Sub(status.UpdatedAt) > time.Duration(status.TTLMS)*time.Millisecond {
		status.State = "stale"
	}
	return status
}

func renderPlain(statuses []Status) string {
	lines := make([]string, 0, len(statuses))
	for _, status := range statuses {
		line := status.Agent + " " + status.State
		if status.Text != "" {
			line += ": " + status.Text
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderTmux(statuses []Status) string {
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, status.Agent+":"+status.State)
	}
	return strings.Join(parts, " | ")
}

func writeJSON(out io.Writer, statuses []Status) error {
	if statuses == nil {
		statuses = []Status{}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(statuses)
}

func statusDir() string {
	if value := os.Getenv("AGENT_STATUS_DIR"); value != "" {
		return value
	}
	if value := os.Getenv("OPEN_SPINNER_DIR"); value != "" {
		return value
	}
	if value := os.Getenv("XDG_RUNTIME_DIR"); value != "" {
		return filepath.Join(value, "open-spinner")
	}
	if value := os.Getenv("XDG_CACHE_HOME"); value != "" {
		return filepath.Join(value, "open-spinner")
	}
	if value, err := os.UserCacheDir(); err == nil && value != "" {
		return filepath.Join(value, "open-spinner")
	}
	return filepath.Join(os.TempDir(), "open-spinner")
}

func statusPath(dir, id string) string {
	return filepath.Join(dir, safeName(id)+".json")
}

func safeName(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// resolveID picks the status ID a `set`/`clear` invocation should use.
// Priority: explicit --id, then env vars, then the hook's own stdin JSON
// session_id (Claude Code and Codex both deliver one, but only via stdin —
// never as an env var), then the resolved tty (already unique per tab),
// and only then the bare agent name. Skipping any of the middle steps
// silently collapses every session of the same agent onto one status
// file, which is the exact bug this chain exists to prevent: one tab's
// hook firing can flip a different, already-finished tab back to busy.
func resolveID(explicit, fallbackAgent string) string {
	if explicit != "" {
		return explicit
	}
	if id := envID(); id != "" {
		return id
	}
	if id := sessionIDFromStdin(); id != "" {
		return id
	}
	if tty := resolveTTY(); tty != "" {
		return tty
	}
	return fallbackAgent
}

func envID() string {
	for _, key := range []string{"OPEN_SPINNER_ID", "AGENT_STATUS_ID", "TMUX_PANE"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

// sessionIDFromStdin extracts session_id from a hook's stdin JSON payload.
// It only ever reads stdin when stdin is provably not an interactive
// terminal (a pipe, as every Claude Code / Codex hook invocation provides),
// so it can never block a manual CLI call, a test, or `open-spinner run`
// wrapping an interactive agent.
func sessionIDFromStdin() string {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil || len(data) == 0 {
		return ""
	}
	var payload struct {
		SessionID string `json:"session_id"`
		// Cursor CLI's hook payload uses conversation_id instead of
		// session_id, but plays the same role: a stable id across a
		// session's turns, present on every hook fired mid-session.
		ConversationID string `json:"conversation_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	if payload.SessionID != "" {
		return payload.SessionID
	}
	return payload.ConversationID
}

// resolveTTY finds the terminal device this process should render status
// into. It never scrapes terminal output — it only reads env vars a hook
// or shell can set, or asks the kernel which tty controls this process.
// An empty result means "no native-tab rendering available," which is a
// valid, documented outcome (e.g. non-interactive shells, some CI runners).
func resolveTTY() string {
	for _, key := range []string{"OPEN_SPINNER_TTY", "AGENT_STATUS_TTY", "TTY", "SSH_TTY"} {
		if tty := concreteTTY(os.Getenv(key)); tty != "" {
			return tty
		}
	}
	if name, err := os.Readlink("/proc/self/fd/0"); err == nil && strings.HasPrefix(name, "/dev/") {
		if tty := concreteTTY(name); tty != "" {
			return tty
		}
	}
	return ttynameFromControllingTerminal()
}

func concreteTTY(name string) string {
	// /dev/tty is a process-local alias; detached renderers need the real pty path.
	if name == "" || name == "/dev/tty" {
		return ""
	}
	return name
}

// ttynameFromControllingTerminal resolves the real device path of this
// process's controlling terminal on platforms with no /proc (macOS, BSD).
// A hook's fd 0 is a JSON pipe, not the terminal, so ps(1) is used instead
// of tty(1), which only reports the device attached to stdin.
func ttynameFromControllingTerminal() string {
	cmd := exec.Command("ps", "-o", "tty=", "-p", fmt.Sprint(os.Getpid()))
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return ttyPathFromPSOutput(out)
}

func ttyPathFromPSOutput(out []byte) string {
	name := strings.TrimSpace(string(out))
	if name == "" || name == "?" || name == "??" {
		return ""
	}
	if strings.HasPrefix(name, "/dev/") {
		return concreteTTY(name)
	}
	if !strings.Contains(name, "/") && !strings.HasPrefix(name, "tty") {
		return ""
	}
	if tty := concreteTTY("/dev/" + name); tty != "" {
		return tty
	}
	return ""
}

func validState(state string) bool {
	return state == "idle" || state == "busy" || state == "attention"
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}
