# Task: Document V0.1 Usage

## Objective
Update user-facing docs to match the implemented V0.1 CLI and testable integrations.

## Context
- README is currently a planning placeholder.
- Keep install promises limited to what exists.
- Go is the chosen implementation framework.

## Changes
1. Update `README.md` with actual Go install/build instructions.
2. Document V0.1 commands and examples.
3. Document status JSON shape.
4. Add a tmux recipe if `print --format tmux` is implemented.
5. Add a short testing section with the actual test command.

## Verification
- Run `go test ./...`.
- Run documented CLI examples against a temp `AGENT_STATUS_DIR`.

## Done
- README no longer describes unimplemented install channels as current behavior.
