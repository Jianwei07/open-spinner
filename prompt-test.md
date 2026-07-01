# open-spinner Test Prompt

Copy/paste this into any coding agent (OpenCode, Pi, Codex, Claude Code, etc.) to test open-spinner.

---

Test open-spinner. Execute these steps with 10 second pauses between each.

## Step 2: Static wrapper test

```sh
cd /Users/jayden77/dev/open-spinner
tmp=$(mktemp -d)
AGENT_STATUS_DIR=$tmp OPEN_SPINNER_TTY=$(tty) ./open-spinner run -- sleep 5
```

Watch Ghostty tab title while this runs. Report:
- What glyph appears?
- Animated or static?
- Clears after 5s?

Wait 10 seconds.

---

## Step 3: Check cleanup

```sh
pgrep -f "open-spinner.*render" | wc -l
```

Should be 0. Report count.

Wait 10 seconds.

---

## Step 4: Detect installed agents

```sh
ls -d ~/.claude ~/.codex ~/.config/opencode 2>/dev/null
```

Report which agent config dirs exist.

Wait 10 seconds.

---

## Step 5: Install open-spinner hooks

```sh
./open-spinner install
```

Report: which agents installed? any errors?

Wait 10 seconds.

---

## Step 6: Trigger agent busy state

Use whatever agent you detected above. Do something that makes it "think" — ask a question, request code generation, etc. This triggers the agent's "busy" hook.

Watch the tab title. Report:
- Glyph appears?
- Animated braille or static?
- Agent name visible?

Wait 10 seconds.

---

## Step 7: Trigger agent attention state

Request something that needs approval/permission from the agent (varies by agent).

Check tab title. Report:
- Shows ⚠ icon?
- Steady (not animated)?

Wait 10 seconds.

---

## Step 8: Agent finishes

Let the agent complete the task.

Check tab title. Report:
- Shows ● idle?
- Clears after ~2s?

Wait 10 seconds.

---

## Step 8b: Two concurrent sessions of the same agent (regression check)

Open a **second** tab/window and start another session of the *same* agent
you used above. This is the scenario that previously made the spinner get
stuck — each session used to collapse onto one shared status file, so one
tab's activity re-armed a different, already-finished tab.

1. In tab 1, let the agent go idle (tab title clears).
2. In tab 2, send a new prompt so it goes busy.
3. Check tab 1's title again.

Report: does tab 1 stay idle/cleared, or does it get flipped back to busy
by tab 2's activity? It must stay idle — if it re-animates, the session
isolation fix regressed.

Wait 10 seconds.

---

## Step 9: Final cleanup

```sh
pgrep -f "open-spinner.*render" | wc -l
```

Should be 0. Report count.

---

## Summary

Report all observations + any crashes/errors.
