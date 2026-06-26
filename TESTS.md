# Tests

Run the project test suite with:

```sh
go test ./...
```

## Required Coverage
- Status write/read using a temp status directory.
- Multiple statuses with deterministic ordering.
- TTL-derived `stale` state.
- `clear` removes the intended status only.
- Output formats: `plain`, `tmux`, `json`.
- Invalid states fail with a nonzero exit code.
- Manual smoke with `AGENT_STATUS_DIR=$(mktemp -d)`.

## CI Target
Eventually run on:

- macOS
- Linux
- Windows

Keep this to one test command until the project proves it needs more.
