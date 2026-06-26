# open-spinner

One tiny status signal for coding agents.

`open-spinner` will let agent CLIs, hooks, and plugins write a shared local status that terminals, tmux, prompts, and tab bars can render.

It will not scrape terminal output. Adapters should emit status directly.

## Status

Planning checkpoint only. Framework choice is intentionally open.

See `.planning/current/HANDOFF.md` for the next-session handoff.

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

## Planned CLI

```sh
open-spinner set busy --agent claude
open-spinner set attention --agent opencode --text "approval needed"
open-spinner set idle --agent codex
open-spinner print --format plain
open-spinner print --format tmux
open-spinner list --format json
open-spinner clear
```

## Planned Install Channels

Start small:

- GitHub Releases binaries
- Homebrew tap
- Scoop for Windows

## Planned tmux Integration

```tmux
set -g status-right '#(open-spinner print --format tmux) %H:%M'
```

## Planned Storage

Status files are JSON and local-only.

Directory order:

1. `$AGENT_STATUS_DIR`
2. `$OPEN_SPINNER_DIR`
3. `$XDG_RUNTIME_DIR/open-spinner`
4. `$XDG_CACHE_HOME/open-spinner`
5. OS user cache dir + `/open-spinner`

Set `$OPEN_SPINNER_ID` or `$AGENT_STATUS_ID` to make multiple hook calls update the same session. If unset, `open-spinner` should use terminal/session env vars like `$TMUX_PANE`, then fall back to the agent name.

## Planned Status JSON

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

## Scope

V0.1 is intentionally small:

- local JSON store
- `set`, `clear`, `list`, `print`
- plain, tmux, and JSON output
- TTL-based stale state

Not doing yet: daemon, universal spinner scraping, terminal-specific plugins, or a rich protocol.
