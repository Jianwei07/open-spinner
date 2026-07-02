# open-spinner

See which of your terminal tabs is busy, idle, or waiting on you — without switching to look.

If you run multiple coding agents (Claude Code, Codex, OpenCode, and others) across tabs in WezTerm, Ghostty, iTerm2, or a WSL2 terminal, there's no way to tell which one needs you without clicking through each tab. `open-spinner` fixes that by writing agent state into the tab title itself, using `OSC 0` — the one tab-title escape sequence every terminal already honors — so it works without tmux and without a per-terminal plugin.

It does not scrape terminal output. Agent hooks/plugins emit status directly, and a small renderer turns that into an animated (or static) tab title.

### Where this fits, honestly

Several mature tmux-native tools already do multi-agent status well (tmux-agent-status, tmux-agent-indicator, and others) — if you live in tmux, those are more feature-complete today. open-spinner's niche is **native terminal tabs, no tmux required**. tmux users aren't left out either: `print --format tmux` (below) still works for a `status-right` segment.

## Status

V0.2: the V0.1 local JSON store, plus native-tab rendering, bundled hook installers for Claude Code/Codex/OpenCode/Qwen Code/Cursor CLI, PATH shims for hookless agents (`pi`/`jcode`/`zai`/`mimo`), and a `run` wrapper for anything else.

## Install

From this repo:

```sh
go build -o open-spinner .
```

Or run without installing:

```sh
go run . --version
```

## Quickstart

```sh
open-spinner install          # auto-detects installed agents and wires their hooks/shims
```

That's it — busy agents now animate their tab title. To uninstall: `open-spinner uninstall`.

If nothing appears, run:

```sh
open-spinner doctor
```

It checks hook/plugin/shim installs, missing binaries, tty resolution, stale status files, renderer locks, and common terminal config overrides such as WezTerm `format-tab-title`.

`pi`, `jcode`, `zai`, and `mimo` don't need manual wrapping — `open-spinner install pi` (or `jcode`/`zai`/`mimo`) installs a PATH shim that does this automatically. For any other agent with no hook system at all, wrap it manually instead:

```sh
open-spinner run -- pi chat
```

## Goal

```txt
coding agent hook/plugin/wrapper -> local status JSON -> tab title / tmux / prompt renderer
```

## States

| State | Meaning | Tab rendering |
|---|---|---|
| `idle` | Agent is ready or done. | static `● agent` |
| `busy` | Agent is thinking, editing, or running tools. | animated braille spinner (static `■ agent` if animation is off) |
| `attention` | User input, approval, auth, conflict, or error needs intervention. | steady `⚠ agent` (never animates, so it stands out from busy) |
| `stale` | Derived when a status is older than its TTL. | static `· agent` |

## CLI

```sh
open-spinner set busy --agent claude
open-spinner set attention --agent opencode --text "approval needed"
open-spinner set idle --agent codex
open-spinner print --format plain
open-spinner print --format tmux
open-spinner list --format json
open-spinner clear
open-spinner install [agent...]
open-spinner uninstall [agent...]
open-spinner doctor
open-spinner run -- <command> [args...]
open-spinner render --id <id> [--tty <path>] [--no-anim]
```

Supported commands:

- `set <idle|busy|attention> --agent <name> [--text <text>] [--id <id>] [--ttl <duration>]` — writing `busy` also auto-spawns a tab-title renderer if a tty is resolvable.
- `clear [--id <id>] [--agent <name>]`
- `list --format json`
- `print --format plain|tmux|json`
- `install [claude|codex|opencode|qwen|cursor|pi|jcode|zai|mimo...]` — with no arguments, auto-detects installed agents: `claude`/`codex`/`opencode`/`qwen`/`cursor` by config-dir presence (`~/.claude`, `~/.codex`, `~/.config/opencode`, `~/.qwen`, `~/.cursor`), `pi`/`jcode`/`zai`/`mimo` by PATH lookup. For the PATH-lookup group (no hook system of their own — or none confirmed yet, see Scope), install writes a PATH shim under `~/.open-spinner/shims/` that wraps the real binary in `run`, plus a one-time PATH line appended to your shell rc file (`~/.zshrc`/`~/.bashrc`/`~/.profile`, chosen by `$SHELL`) — open a new shell or re-source your rc file for it to take effect. Safe to re-run; only ever touches entries it wrote itself.
- `uninstall [claude|codex|opencode|qwen|cursor|pi|jcode|zai|mimo...]` — reverses `install`. With no arguments, tries all known agents (a no-op wherever nothing was installed).
- `doctor` (or `--doctor`) — read-only diagnostics for install wiring, tty resolution, status files, renderer locks, and terminal tab-title overrides.
- `run [--agent name] [--id id] -- <command> [args...]` — for agents with no hook system: marks the whole run `busy`, clears on exit. Coarse (whole process lifetime, not per-turn), but honest — no scraping involved.
- `render --id <id> [--tty <path>] [--no-anim] [--restore <title>]` — the tab-title renderer itself. Normally spawned automatically by `set busy` or `run`; only call directly for debugging.
- `version` or `--version`

Future install channels can add GitHub release binaries, Homebrew, and Scoop after release automation exists.

## Native tab rendering

`open-spinner render` binds one ticker to one tty and writes `OSC 0` title updates (`\e]0;<glyph> <agent>\a`) — the same escape iTerm2, WezTerm, Ghostty, Windows Terminal, Alacritty, and kitty all already support. Design choices that keep it from becoming the fragile part:

- **One renderer per tty, ever.** A lockfile keyed on the tty path means repeated hook fires (every prompt, every tool call) never spawn duplicates — later `set` calls just update the store, and the running renderer picks up the change on its next tick.
- **Self-terminating, not a daemon.** It's lazily spawned on the first `busy`, and exits on its own: after the idle-grace window, immediately on an explicit `clear`, or the moment a tty write fails (tab closed). No orphan processes to clean up.
- **Degrades to static automatically** under `$TMUX` (the outer tab isn't reachable from a tmux pane the way `print --format tmux` is), with `--no-anim`, or with `OPEN_SPINNER_ANIM=0` — the same poll loop still runs (it needs to notice `idle`/`clear` regardless), it just writes the static glyph once and skips re-writing on every tick instead of cycling spinner frames.
- Ghostty only supports plain tab-title text (its maintainers have declined a richer badge feature); the glyph still shows, just without iTerm2-style badge styling.
- Custom terminal tab formatters can override OSC titles. In WezTerm, a `format-tab-title` handler must preserve the pane title if you want open-spinner's native title to be visible; `open-spinner doctor` warns when it detects this setup.

## tmux Integration

If you do use tmux, use the plain-text renderer instead of native-tab rendering:

```tmux
set -g status-right '#(open-spinner print --format tmux) %H:%M'
```

## Storage

Status files are JSON and local-only.

Directory order:

1. `$AGENT_STATUS_DIR`
2. `$OPEN_SPINNER_DIR`
3. `$XDG_RUNTIME_DIR/open-spinner`
4. `$XDG_CACHE_HOME/open-spinner`
5. OS user cache dir + `/open-spinner`

Set `$OPEN_SPINNER_ID` or `$AGENT_STATUS_ID` to make multiple hook calls update the same session. If unset, `open-spinner` uses `$TMUX_PANE`, then falls back to the agent name.

The tty a status renders into is resolved the same way: `$OPEN_SPINNER_TTY`, then `$AGENT_STATUS_TTY`, `$TTY`, `$SSH_TTY`, then the process's own controlling terminal. An empty result just means no native-tab rendering for that session — the JSON store and `print`/`list` still work.

## Status JSON

```json
{
  "v": 1,
  "id": "pane-or-session-id",
  "agent": "claude",
  "state": "busy",
  "text": "running tool",
  "cwd": "/repo",
  "pid": 12345,
  "tty": "/dev/ttys003",
  "updated_at": "2026-06-26T12:00:00Z",
  "ttl_ms": 300000
}
```

`stale` is derived when `updated_at + ttl_ms` is older than now. Agents write only `idle`, `busy`, or `attention`.

## Test

```sh
go test ./...
```

See [TESTS.md](TESTS.md) for the full breakdown (unit, PTY integration, install idempotency, fake-agent end-to-end, manual smoke matrix, and soak checks).

Manual smoke:

```sh
tmp=$(mktemp -d)
AGENT_STATUS_DIR=$tmp go run . set busy --agent opencode
AGENT_STATUS_DIR=$tmp go run . print --format plain
AGENT_STATUS_DIR=$tmp go run . clear
```

Manual smoke for the `pi`/`jcode` shim install:

```sh
tmp=$(mktemp -d)
mkdir -p "$tmp/home" "$tmp/fakebin"
printf '#!/bin/sh\necho "fake-pi ran with args: $@"\n' > "$tmp/fakebin/pi"
chmod +x "$tmp/fakebin/pi"

HOME="$tmp/home" PATH="$tmp/fakebin:$PATH" go run . install pi
ls -l "$tmp/home/.open-spinner/shims/pi"      # exists, executable, managed-by marker
cat "$tmp/home/.zshrc"                        # (or .bashrc/.profile) PATH block present once

AGENT_STATUS_DIR="$tmp/status" HOME="$tmp/home" PATH="$tmp/fakebin:$PATH" \
  "$tmp/home/.open-spinner/shims/pi" hello world   # prints "fake-pi ran with args: hello world"
```

## Scope

What's here:

- local JSON store — `set`, `clear`, `list`, `print` (plain/tmux/JSON), TTL-based stale state
- native tab-title rendering via OSC 0, animated with a static fallback
- bundled hook installers for Claude Code, Codex, OpenCode, Qwen Code, and Cursor CLI
- a `run` wrapper for agents without a hook system, plus installable PATH shims for `pi`/`jcode`/`zai`/`mimo`

Deliberate limits, not oversights:

- No auto-detection of *arbitrary* agent CLIs — hooks are bundled for the agents above with confirmed schemas; any other hookless agent still goes through `run` manually (whole-process-lifetime granularity) or needs its own adapter.
- **Cursor CLI has no `attention` state.** Its hook events don't include a clean "needs user input" analog — the closest ones (`beforeShellExecution`, `beforeMCPExecution`) are approval-decision hooks that likely expect a specific allow/deny JSON on stdout, and open-spinner's fire-and-forget `set` doesn't produce one. Wiring that blind risks silently blocking your own shell commands, which is worse than a missing tab glyph — so Cursor only gets `busy`/`idle` for now.
- **`zai` and `mimo` (Xiaomi's MiMo Code) are PATH shims, not hook installers**, even though MiMo is a fork of OpenCode and may retain its plugin system — its plugin directory convention isn't confirmed anywhere, and guessing it wrong would silently never load (this is exactly how the Codex `Notification` bug happened: an unverified event name that just never fires). The shim is coarse but always correct; a native MiMo plugin installer is a good follow-up once someone confirms the actual plugin path.
- No terminal-output scraping, ever.
- No animation under tmux — static/`print --format tmux` there instead.
