# Tests

Run the project test suite with:

```sh
go test ./...
```

## Required Coverage: store + CLI (V0.1)
- Status write/read using a temp status directory.
- Multiple statuses with deterministic ordering.
- TTL-derived `stale` state.
- `clear` removes the intended status only.
- Output formats: `plain`, `tmux`, `json`.
- Invalid states fail with a nonzero exit code.
- Manual smoke with `AGENT_STATUS_DIR=$(mktemp -d)`.

## Required Coverage: native-tab renderer (V0.2)

All layers below run in `go test ./...` except the manual smoke matrix.

1. **Unit** (`osc_test.go`): frame sequencer wraparound, state->glyph
   mapping (busy animates, attention is steady, idle/stale are static),
   byte-exact OSC encoding, animation degrade reasons (`--no-anim`,
   `OPEN_SPINNER_ANIM=0`, `$TMUX`).
2. **Renderer/PTY integration** (`render_test.go`): a real PTY pair proves
   the busy state actually cycles distinct frames; a fake tty writer
   proves lifecycle behavior deterministically — exit after the idle-grace
   window, immediate exit on explicit `clear`, clean exit (not a hard
   error) when the tty write fails (closed tab), title restored on
   signal. Separate tests cover the tty lockfile: single instance per
   tty, and stale-lock reclaim when the previous holder's pid is dead.
3. **Install idempotency** (`install_test.go`): installing Claude/Codex/
   OpenCode hooks twice against a temp `$HOME` leaves exactly one managed
   hook/entry per event; unrelated hooks and top-level config keys are
   preserved; uninstall removes only entries/files tagged as
   open-spinner-managed and is a no-op when nothing was installed;
   installing OpenCode's plugin refuses to clobber a pre-existing
   unmanaged file.
4. **Fake-agent end-to-end** (`tests/fake_agent_test.go`): replays the
   exact CLI sequence a real hook config fires — `set busy` (UserPromptSubmit)
   -> `set attention` (Notification) -> `set idle` (Stop) -> `clear`
   (session end) — against a real PTY, with no real agent or API key
   involved. Proves the full chain: status write -> auto-spawned renderer
   -> OSC bytes on the tty -> clean shutdown on clear.

Run just the new layers:

```sh
go test . -run 'Frame|Glyph|OSC|Animation|RenderLoop|AcquireTTYLock|Install|Uninstall' -v
go test ./tests/... -run FakeAgent -v
```

## Manual smoke matrix (native tabs — the one thing automation can't prove)

Run once with a real Claude Code session, once with a hookless agent via
`open-spinner run -- <agent>`:

| Terminal | busy animates | attention is steady `⚠` | clears on idle/clear | title restored on exit |
|---|---|---|---|---|
| iTerm2 | ☐ | ☐ | ☐ | ☐ |
| WezTerm | ☐ | ☐ | ☐ | ☐ |
| Ghostty | ☐ | ☐ | ☐ | ☐ |
| Windows Terminal (WSL2) | ☐ | ☐ | ☐ | ☐ |

Record a short GIF per terminal — proof for the checklist and material
for the README.

## Soak (stability bar)

Since "animated" is the part most likely to become "fragile," check
periodically, not just once:

```sh
tmp=$(mktemp -d)
AGENT_STATUS_DIR=$tmp OPEN_SPINNER_TTY=$(tty) go run . set busy --agent soak-test --id soak
# leave running 30+ min; watch:
ps -o pid,pcpu,rss,command -p $(pgrep -f 'open-spinner render.*soak')
# fire 100 rapid updates and confirm only one renderer process exists throughout:
for i in $(seq 1 100); do AGENT_STATUS_DIR=$tmp go run . set busy --agent soak-test --id soak; done
pgrep -fc 'open-spinner render.*soak'   # expect 1
AGENT_STATUS_DIR=$tmp go run . clear --id soak
sleep 1
pgrep -fc 'open-spinner render.*soak'   # expect 0 (no orphan)
```

## CI Target
Eventually run on:

- macOS
- Linux
- Windows

The PTY-based tests (layers 2 and 4 above) are Unix-specific (`creack/pty`,
`syscall.Kill`); a Windows CI target will need either a Windows-specific
renderer test path or to skip those two layers there.
