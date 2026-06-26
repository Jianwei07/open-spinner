# Task: Add Tests

## Objective
Add a small test suite that proves the core status contract works.

## Context
- Tests should use temp directories and no real user cache.
- Avoid broad fixtures or framework ceremony.

## Changes
1. Add tests for writing and listing one status.
2. Add tests for multiple agent statuses and deterministic ordering.
3. Add tests for TTL-derived `stale` state.
4. Add tests for `clear` behavior.
5. Add tests for `plain`, `tmux`, and `json` output.
6. Add tests for invalid state handling.

## Verification
- Run the framework-specific test command added by task `01.01`.

## Done
- Tests fail if state transitions, storage, or output formats break.
