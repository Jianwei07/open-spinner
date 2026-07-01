# open-spinner Visual Tab Test

Paste this into any coding agent CLI (Claude Code, Codex, OpenCode, pi, jcode, etc). Works the same regardless of terminal emulator or which agent is reading it — it drives `open-spinner` directly with synthetic states instead of relying on that agent's own hooks.

---

You are testing whether `open-spinner` correctly renders three states into this terminal tab's title: **busy** (animated spinner), **attention** (steady `!`/`⚠`), and **idle/stop** (clears). Run the steps below in order, pausing so each state is visible long enough to eyeball, then report what you saw. Clean up everything you created when done, even if a step fails.

## Setup

```sh
BIN=""
if [ -x ./open-spinner ]; then BIN=./open-spinner
elif command -v open-spinner >/dev/null 2>&1; then BIN=open-spinner
else BIN="go run ."
fi

TMP=$(mktemp -d)
export AGENT_STATUS_DIR="$TMP"
export OPEN_SPINNER_TTY=$(tty)
ID=spin-visual-test
```

## Step 1: Busy — spinner should animate

```sh
$BIN set busy --agent $ID --id $ID
```

Watch the tab title for 8 seconds. Report:
- Glyph + agent name visible?
- Is it animating (cycling braille frames), not static?

## Step 2: Attention — steady `!`/`⚠`

```sh
$BIN set attention --agent $ID --id $ID --text "needs you"
```

Watch the tab title for 8 seconds. Report:
- Shows `⚠`?
- Steady, NOT animated (this is how it must differ from busy)?

## Step 3: Idle then clear

```sh
$BIN set idle --agent $ID --id $ID
```

Watch for ~3 seconds. Report:
- Shows `●` idle?
- Title clears/restores on its own shortly after (idle-grace window)?

If it hasn't cleared on its own, force it:

```sh
$BIN clear --id $ID
```

Confirm the title is back to normal.

## Cleanup — do this even if a step above failed

```sh
$BIN clear --id $ID
pgrep -f "open-spinner.*render.*$ID" | wc -l   # should be 0
rm -rf "$TMP"
```

Report the leftover-process count (must be 0) and confirm the temp dir was removed.

## Report

Summarize: did busy/attention/idle each look visibly distinct (spin vs steady `⚠` vs clear)? Any stuck title, leftover process, or error along the way?
