# Decisions

## Accepted
- Project name: `open-spinner`.
- License: MIT.
- V0.1 uses direct status emission from hooks/plugins/wrappers, not terminal-output scraping.
- Core user states: `idle`, `busy`, `attention`.
- `stale` is derived by TTL, not emitted as a primary agent state.
- Local JSON files are the canonical V0.1 status store.
- No daemon, network, or telemetry in V0.1.
- Implementation framework: Go, using the standard library for the V0.1 CLI, JSON store, and tests.
- First terminal renderer recipe: tmux via `open-spinner print --format tmux`.

## Pending
- First agent adapter.
