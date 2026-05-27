package play

import (
	"os"
	"strconv"
	"strings"
)

// evKind tags a decoded terminal-input event.
type evKind int

const (
	evByte  evKind = iota // a ground byte (printable or control) in ev.b
	evArrow               // an arrow key; ev.b is 'A','B','C', or 'D'
	evMouse               // a mouse button event at ev.mx,ev.my (1-based)
)

// event is one decoded input from the terminal.
type event struct {
	kind  evKind
	b     byte // evByte: the byte; evArrow: the arrow final byte
	mx    int  // evMouse: column (1-based)
	my    int  // evMouse: row (1-based)
	press bool // evMouse: left-button press (vs release/other)
}

type inputState int

const (
	stGround inputState = iota
	stEsc
	stCSI
	stSS3
	stOSC
	stOSCEsc
)

// inputParser turns a raw terminal byte stream into events. It recognises the
// keys the player needs (printable/control bytes, arrows, SGR mouse) and
// swallows every other escape sequence — notably the terminal's own replies to
// query sequences echoed during replay (Device Attributes, cursor reports, OSC
// color answers), which are all ESC-prefixed and must not be seen as input.
type inputParser struct {
	state  inputState
	csiBuf []byte // CSI parameter/intermediate bytes, for arrow and mouse decode
}

func (p *inputParser) feed(b byte, out chan<- event) {
	switch p.state {
	case stGround:
		if b == 0x1b {
			p.state = stEsc
			return
		}
		out <- event{kind: evByte, b: b}
	case stEsc:
		switch b {
		case '[':
			p.state = stCSI
			p.csiBuf = p.csiBuf[:0]
		case 'O': // SS3 — application-keypad arrows (ESC O A..D)
			p.state = stSS3
		case ']':
			p.state = stOSC
		default:
			p.state = stGround
		}
	case stCSI:
		if b >= 0x40 && b <= 0x7e { // final byte
			p.dispatchCSI(b, out)
			p.state = stGround
		} else {
			p.csiBuf = append(p.csiBuf, b)
		}
	case stSS3:
		if b == 'A' || b == 'B' || b == 'C' || b == 'D' {
			out <- event{kind: evArrow, b: b}
		}
		p.state = stGround
	case stOSC:
		switch b {
		case 0x07: // BEL terminates OSC
			p.state = stGround
		case 0x1b: // possible ST (ESC \)
			p.state = stOSCEsc
		}
	case stOSCEsc:
		if b == '\\' {
			p.state = stGround
		} else {
			// Back-to-back sequence: this ESC began a new one. Re-dispatch.
			p.state = stEsc
			p.feed(b, out)
		}
	}
}

func (p *inputParser) dispatchCSI(final byte, out chan<- event) {
	// Bare arrow: no parameters and an A/B/C/D final.
	if len(p.csiBuf) == 0 {
		switch final {
		case 'A', 'B', 'C', 'D':
			out <- event{kind: evArrow, b: final}
		}
		return
	}
	// SGR mouse: ESC [ < btn ; col ; row (M=press | m=release).
	if (final == 'M' || final == 'm') && p.csiBuf[0] == '<' {
		parts := strings.Split(string(p.csiBuf[1:]), ";")
		if len(parts) != 3 {
			return
		}
		btn, e1 := strconv.Atoi(parts[0])
		col, e2 := strconv.Atoi(parts[1])
		row, e3 := strconv.Atoi(parts[2])
		if e1 != nil || e2 != nil || e3 != nil {
			return
		}
		// Left button, no motion/wheel bits set, on a press ('M').
		leftPress := final == 'M' && btn&0x63 == 0
		out <- event{kind: evMouse, mx: col, my: row, press: leftPress}
	}
}

// readInput reads stdin and emits decoded events until stdin closes. Parser
// state persists across reads so an escape sequence split across reads still
// decodes correctly.
func readInput(out chan<- event) {
	var p inputParser
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		for i := 0; i < n; i++ {
			p.feed(buf[i], out)
		}
		if err != nil {
			close(out)
			return
		}
	}
}
