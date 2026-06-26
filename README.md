# open-spinner

One tiny status signal for coding agents.

`open-spinner` lets agent CLIs, hooks, and plugins write a shared local status that terminals, tmux, prompts, and tab bars can render.

It does not scrape terminal output. Adapters should emit status directly.

## Status

V0.1 Go CLI.

## Install

From this repo:

```sh
go build -o open-spinner .
```

Or run without installing:

```sh
go run . --version
```

## Goal

```txt
coding agent hook/plugin/wrapper -> local status JSON -> terminal/tmux/prompt renderer
```

## States

| State | Meaning |
|---|---|
| `idle` | Agent is ready or done. |
| `busy` | Agent is thinking, editing, or running tools. |
| `attention` | User input, approval, auth, conflict, or error needs intervention. |
| `stale` | Derived when a status is older than its TTL. |

## CLI

```sh
open-spinner set busy --agent claude
open-spinner set attention --agent opencode --text "approval needed"
open-spinner set idle --agent codex
open-spinner print --format plain
open-spinner print --format tmux
open-spinner list --format json
open-spinner clear
```

Supported commands:

- `set <idle|busy|attention> --agent <name> [--text <text>] [--id <id>] [--ttl <duration>]`
- `clear [--id <id>] [--agent <name>]`
- `list --format json`
- `print --format plain|tmux|json`
- `version` or `--version`

Future install channels can add GitHub release binaries, Homebrew, and Scoop after release automation exists.

## tmux Integration

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
  "updated_at": "2026-06-26T12:00:00Z",
  "ttl_ms": 300000
}
```

`stale` is derived when `updated_at + ttl_ms` is older than now. Agents write only `idle`, `busy`, or `attention`.

## Test

```sh
go test ./...
```

Manual smoke:

```sh
tmp=$(mktemp -d)
AGENT_STATUS_DIR=$tmp go run . set busy --agent opencode
AGENT_STATUS_DIR=$tmp go run . print --format plain
AGENT_STATUS_DIR=$tmp go run . clear
```

## Scope

V0.1 is intentionally small:

- local JSON store
- `set`, `clear`, `list`, `print`
- plain, tmux, and JSON output
- TTL-based stale state

Not doing yet: daemon, universal spinner scraping, terminal-specific plugins, or a rich protocol.
