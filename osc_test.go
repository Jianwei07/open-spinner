package main

import "testing"

func TestFrameForTickCyclesAndWraps(t *testing.T) {
	if got := frameForTick(0); got != brailleFrames[0] {
		t.Fatalf("frame(0) = %q, want %q", got, brailleFrames[0])
	}
	if got := frameForTick(3); got != brailleFrames[3] {
		t.Fatalf("frame(3) = %q, want %q", got, brailleFrames[3])
	}
	// wraps at len(brailleFrames) == 10
	if got, want := frameForTick(10), frameForTick(0); got != want {
		t.Fatalf("frame(10) = %q, want wrap to frame(0) = %q", got, want)
	}
	if got, want := frameForTick(23), brailleFrames[3]; got != want {
		t.Fatalf("frame(23) = %q, want %q", got, want)
	}
	// never panics on negative input
	if got := frameForTick(-4); got == "" {
		t.Fatalf("frame(-4) returned empty string")
	}
}

func TestGlyphForState(t *testing.T) {
	cases := []struct {
		name         string
		state        string
		animate      bool
		wantText     string
		wantAnimated bool
	}{
		{"busy animated", "busy", true, "⠋ claude", true},
		{"busy static fallback", "busy", false, "■ claude", false},
		{"attention always steady even if animate requested", "attention", true, "⚠ claude", false},
		{"idle is static", "idle", true, "● claude", false},
		{"stale is dim/static", "stale", true, "· claude", false},
		{"unknown state passes through agent name", "weird", true, "claude", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, animated := glyphForState(c.state, "claude", 0, c.animate)
			if text != c.wantText {
				t.Errorf("text = %q, want %q", text, c.wantText)
			}
			if animated != c.wantAnimated {
				t.Errorf("animated = %v, want %v", animated, c.wantAnimated)
			}
		})
	}
}

func TestGlyphForStateBusyAnimatedChangesPerTick(t *testing.T) {
	text0, _ := glyphForState("busy", "codex", 0, true)
	text1, _ := glyphForState("busy", "codex", 1, true)
	if text0 == text1 {
		t.Fatalf("expected different frames at tick 0 and 1, got %q both times", text0)
	}
}

func TestOSCTitleBytesExact(t *testing.T) {
	got := oscTitle("⠙ claude")
	want := []byte("\x1b]0;⠙ claude\a")
	if string(got) != string(want) {
		t.Fatalf("oscTitle bytes = %q, want %q", got, want)
	}
}

func TestOSCTitleEmpty(t *testing.T) {
	got := oscTitle("")
	want := []byte("\x1b]0;\a")
	if string(got) != string(want) {
		t.Fatalf("oscTitle(\"\") bytes = %q, want %q", got, want)
	}
}

func TestAnimationEnabledDegradeReasons(t *testing.T) {
	cases := []struct {
		name        string
		noAnimFlag  bool
		envAnim     string
		tmuxEnv     string
		wantEnabled bool
	}{
		{"default enabled", false, "", "", true},
		{"explicit --no-anim flag disables", true, "", "", false},
		{"OPEN_SPINNER_ANIM=0 disables", false, "0", "", false},
		{"inside tmux disables", false, "", "/tmp/tmux-1000/default,1234,0", false},
		{"OPEN_SPINNER_ANIM=1 does not disable", false, "1", "", true},
		{"all three reasons still disables", true, "0", "/tmux", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := animationEnabled(c.noAnimFlag, c.envAnim, c.tmuxEnv)
			if got != c.wantEnabled {
				t.Errorf("animationEnabled(%v, %q, %q) = %v, want %v", c.noAnimFlag, c.envAnim, c.tmuxEnv, got, c.wantEnabled)
			}
		})
	}
}
