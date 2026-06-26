# Task: Implement CLI Commands

## Objective
Implement the V0.1 CLI surface for setting, clearing, listing, and printing agent status.

## Context
- Depends on task `01.02` status store.
- Keep command surface small.

## Changes
1. Implement `open-spinner set <idle|busy|attention> --agent <name> [--text <text>] [--id <id>] [--ttl <duration>]`.
2. Implement `open-spinner clear [--id <id>] [--agent <name>]`.
3. Implement `open-spinner list --format json`.
4. Implement `open-spinner print --format plain|tmux|json`.
5. Implement `open-spinner version` or `--version`.

## Verification
- Run the framework-specific test command added by task `01.01`.
- Run manual CLI smoke checks using a temp `AGENT_STATUS_DIR`.

## Done
- CLI supports the planned V0.1 commands.
- Invalid states and missing required flags fail with nonzero exit codes.
