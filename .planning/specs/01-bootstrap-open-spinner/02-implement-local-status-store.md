# Task: Implement Local Status Store

## Objective
Implement the local JSON status store used by all V0.1 commands.

## Context
- Canonical status storage is local JSON files.
- Framework-specific files are decided in task `01.01` before executing this task.
- No daemon, network, or telemetry.

## Changes
1. Add the framework-specific status model.
2. Add status directory resolution in this order: `$AGENT_STATUS_DIR`, `$OPEN_SPINNER_DIR`, `$XDG_RUNTIME_DIR/open-spinner`, `$XDG_CACHE_HOME/open-spinner`, OS cache fallback.
3. Add atomic-ish status writes appropriate for the chosen framework.
4. Add status reads that ignore malformed files instead of failing the whole list.
5. Add TTL handling so expired statuses render as `stale`.

## Verification
- Run the framework-specific test command added by task `01.01`.
- Confirm temp-directory tests cover write, read, malformed file ignore, and TTL stale behavior.

## Done
- Status store works without global state or user-specific paths in tests.
