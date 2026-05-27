package play

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	// SGR mouse reporting: enable button events with extended (SGR) coordinates.
	mouseEnable  = "\x1b[?1000h\x1b[?1006h"
	mouseDisable = "\x1b[?1006l\x1b[?1000l"

	clrReset = "\x1b[0m"
	clrBold  = "\x1b[1m"
	clrGreen = "\x1b[32m"
	clrDim   = "\x1b[2m"
	clrCyan  = "\x1b[36m"
)

// termSize returns the terminal width and height, with 80x24 fallback.
func termSize() (int, int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

// formatClock renders seconds as MM:SS (or HMM:SS-style overflow past 99 min).
func formatClock(secs float64) string {
	if secs < 0 {
		secs = 0
	}
	t := int(secs + 0.5)
	return fmt.Sprintf("%02d:%02d", t/60, t%60)
}

// parseClock parses a goto target: "SS", "MM:SS", or "M:SS".
func parseClock(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		m, e1 := strconv.Atoi(s[:i])
		sec, e2 := strconv.Atoi(s[i+1:])
		if e1 != nil || e2 != nil || m < 0 || sec < 0 || sec >= 60 {
			return 0, false
		}
		return float64(m*60 + sec), true
	}
	sec, e := strconv.Atoi(s)
	if e != nil || sec < 0 {
		return 0, false
	}
	return float64(sec), true
}

func formatSpeed(s float64) string {
	return strconv.FormatFloat(s, 'g', -1, 64) + "x"
}

func faster(s float64) float64 {
	if s < maxSpeed {
		return s * 2
	}
	return s
}

func slower(s float64) float64 {
	if s > minSpeed {
		return s / 2
	}
	return s
}

// renderBar builds the colored status-bar line and returns it with the 1-based
// column of the first progress-bar cell and the bar width, for mouse seeking.
func renderBar(width int, vt, lastT, speed float64, paused, inGoto bool, gotoBuf string) (line string, barCol, barW int) {
	icon := "▶" // ▶
	if paused {
		icon = "⏸" // ⏸
	}
	left := fmt.Sprintf("%s %s / %s ", icon, formatClock(vt), formatClock(lastT))

	var right string
	if inGoto {
		right = fmt.Sprintf(" goto %s_  (enter jump · any other key cancel)", gotoBuf)
	} else {
		pct := 0.0
		if lastT > 0 {
			pct = vt / lastT * 100
		}
		right = fmt.Sprintf(" %3.0f%%  %s   ←/→ seek · g goto · space play · q quit", pct, formatSpeed(speed))
	}

	barW = width - utf8.RuneCountInString(left) - utf8.RuneCountInString(right)
	if barW < 10 {
		barW = 10
	}
	filled := 0
	if lastT > 0 {
		filled = int(float64(barW-1)*vt/lastT + 0.5)
	}
	if filled > barW-1 {
		filled = barW - 1
	}
	if filled < 0 {
		filled = 0
	}
	// Thin-line scrubber: heavy line played, a knob at the head, light line ahead.
	bar := clrGreen + strings.Repeat("━", filled) + clrReset +
		clrCyan + "●" + clrReset +
		clrDim + strings.Repeat("─", barW-filled-1) + clrReset
	line = clrBold + left + clrReset + bar + clrCyan + right + clrReset
	barCol = utf8.RuneCountInString(left) + 1 // first bar cell follows the left text
	return line, barCol, barW
}

// drawStatus paints the status bar on the bottom row without moving the cursor,
// returning the bar's column, width, and row for mouse hit-testing.
func drawStatus(vt, lastT, speed float64, paused, inGoto bool, gotoBuf string) (barCol, barW, row int) {
	w, h := termSize()
	line, bc, bw := renderBar(w, vt, lastT, speed, paused, inGoto, gotoBuf)
	// Save cursor, go to the bottom row, clear it, disable autowrap (so a line
	// that reaches the right edge can't trigger a scroll that duplicates
	// content), draw, re-enable autowrap, restore cursor.
	fmt.Fprintf(os.Stdout, "\x1b7\x1b[%d;1H\x1b[2K\x1b[?7l%s\x1b[?7h\x1b8", h, line)
	return bc, bw, h
}
