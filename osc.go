package main

// OSC 0 sets both window and tab title. It's the one cross-terminal
// primitive: iTerm2, WezTerm, Ghostty, Windows Terminal, Alacritty, and
// kitty all honor it, so a single code path lights up any terminal's tab
// without a per-terminal plugin.
const oscTitlePrefix = "\x1b]0;"
const oscTitleSuffix = "\a"

var braillFrames = [10]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// frameForTick returns the braille spinner glyph for a given tick, cycling
// through the full set. Kept as a pure function so the cadence/wraparound
// is unit-testable without a ticker or a terminal.
func frameForTick(tick int) string {
	if tick < 0 {
		tick = -tick
	}
	return braillFrames[tick%len(braillFrames)]
}

// glyphForState renders the tab-title text for a status. It returns
// whether the result is an animated frame (true only for busy+animate),
// so callers/tests can assert animation actually happens only when
// expected. attention is always steady so it visually stands out from a
// cycling busy spinner.
func glyphForState(state, agent string, tick int, animate bool) (text string, animated bool) {
	switch state {
	case "busy":
		if animate {
			return frameForTick(tick) + " " + agent, true
		}
		return "■ " + agent, false
	case "attention":
		return "⚠ " + agent, false
	case "idle":
		return "● " + agent, false
	case "stale":
		return "· " + agent, false
	default:
		return agent, false
	}
}

// oscTitle encodes a tab-title write. Byte-exact and side-effect free so
// it can be asserted against directly in unit tests.
func oscTitle(title string) []byte {
	return []byte(oscTitlePrefix + title + oscTitleSuffix)
}

// animationEnabled centralizes every reason the renderer must degrade to
// a single static write instead of a ticking spinner:
//   - explicit --no-anim flag
//   - OPEN_SPINNER_ANIM=0
//   - running inside tmux, where the outer tab title isn't reachable from
//     a pane process the way pane-title/status-right are; animating here
//     would just repaint tmux's own pane title uselessly.
func animationEnabled(noAnimFlag bool, envAnim string, tmuxEnv string) bool {
	if noAnimFlag {
		return false
	}
	if envAnim == "0" {
		return false
	}
	if tmuxEnv != "" {
		return false
	}
	return true
}
