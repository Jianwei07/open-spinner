# open-spinner

## Core Value
Give coding agents one common local status signal so terminals, tmux, prompts, and tab bars can show which agent is idle, busy, blocked, or stale.

## Product Shape
`open-spinner` is a tiny CLI plus convention. Agent hooks/plugins/wrappers write local JSON status. Renderers read that JSON and display simple status.

## Constraints
- No terminal-output scraping for V0.1.
- No daemon for V0.1.
- No network or telemetry.
- Framework choice is open until the next session.
- Keep implementation dependency-light.

## Accepted Direction
Build an adapter-based status bridge, not a universal spinner detector.

## Primary Users
- Developers running multiple coding agents in tmux, WezTerm, or iTerm2.
- Staff engineers supervising parallel agent tasks.
- Remote/headless users who need blocked/done visibility without checking every pane.
