package play

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFormatClock(t *testing.T) {
	cases := map[float64]string{0: "00:00", 5: "00:05", 65: "01:05", 3599: "59:59", 3600: "60:00"}
	for in, want := range cases {
		if got := formatClock(in); got != want {
			t.Errorf("formatClock(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestParseClock(t *testing.T) {
	ok := map[string]float64{"0": 0, "42": 42, "1:05": 65, "10:00": 600, "00:30": 30}
	for in, want := range ok {
		if got, valid := parseClock(in); !valid || got != want {
			t.Errorf("parseClock(%q) = %v,%v want %v,true", in, got, valid, want)
		}
	}
	for _, bad := range []string{"", "1:99", "abc", ":", "-5", "1:2:3"} {
		if got, valid := parseClock(bad); valid {
			t.Errorf("parseClock(%q) = %v,true want invalid", bad, got)
		}
	}
}

func TestFormatSpeed(t *testing.T) {
	cases := map[float64]string{1: "1x", 2: "2x", 0.5: "0.5x", 0.25: "0.25x"}
	for in, want := range cases {
		if got := formatSpeed(in); got != want {
			t.Errorf("formatSpeed(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFasterSlowerClamp(t *testing.T) {
	if got := faster(maxSpeed); got != maxSpeed {
		t.Errorf("faster at max = %v, want %v", got, maxSpeed)
	}
	if got := slower(minSpeed); got != minSpeed {
		t.Errorf("slower at min = %v, want %v", got, minSpeed)
	}
	if got := faster(1); got != 2 {
		t.Errorf("faster(1) = %v, want 2", got)
	}
}

// visibleLen counts runes outside ANSI escape sequences. A CSI sequence is
// ESC '[' params/intermediates then a final byte in 0x40..0x7e.
func visibleLen(s string) int {
	rs := []rune(s)
	n := 0
	for i := 0; i < len(rs); i++ {
		if rs[i] == 0x1b {
			i++
			if i < len(rs) && rs[i] == '[' {
				i++
				for i < len(rs) && !(rs[i] >= '@' && rs[i] <= '~') {
					i++
				}
				// rs[i] is the final byte; the loop's i++ skips it
			}
			continue
		}
		n++
	}
	return n
}

func TestRenderBarGeometry(t *testing.T) {
	width := 100
	line, barCol, barW := renderBar(width, 30, 120, 1, false, false, "")
	if barW < 10 {
		t.Errorf("barW = %d, want >= 10", barW)
	}
	// The first bar cell sits just after the left text.
	if barCol < 2 {
		t.Errorf("barCol = %d, want >= 2", barCol)
	}
	// Played portion (heavy line) should be about a quarter (30/120) of the bar.
	filled := strings.Count(line, "━")
	wantFilled := int(float64(barW-1)*0.25 + 0.5)
	if filled != wantFilled {
		t.Errorf("filled=%d want %d (barW=%d)", filled, wantFilled, barW)
	}
	if !strings.Contains(line, "●") {
		t.Errorf("bar missing position knob: %q", line)
	}
	// Visible width should not exceed the terminal width.
	if vl := visibleLen(line); vl > width {
		t.Errorf("visible line width %d exceeds %d", vl, width)
	}
	_ = utf8.RuneCountInString(line)
}

func TestRenderBarGotoField(t *testing.T) {
	line, _, _ := renderBar(100, 0, 60, 1, true, true, "1:23")
	if !strings.Contains(line, "goto 1:23") {
		t.Errorf("goto bar missing input field: %q", line)
	}
}
