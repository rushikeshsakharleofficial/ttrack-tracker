// Package play — scrollback.go: line buffer for the scrollback viewer.
package play

import (
	"strings"
	"unicode/utf8"
)

// scrollStep is lines scrolled per key/wheel action.
const scrollStep = 3

// lineBuf accumulates terminal-output lines for the scrollback viewer.
// Watches \n and \r to delimit lines, strips cursor-movement escape
// sequences, preserves SGR colour/style sequences.
type lineBuf struct {
	lines []string
	cur   strings.Builder
}

// feed processes raw terminal output data and appends completed lines.
func (b *lineBuf) feed(data string) {
	i := 0
	for i < len(data) {
		c := data[i]
		switch {
		case c == '\n':
			b.lines = append(b.lines, b.cur.String())
			b.cur.Reset()
			i++
		case c == '\r':
			if i+1 < len(data) && data[i+1] == '\n' {
				i++ // CR before LF: skip CR
				continue
			}
			b.cur.Reset() // standalone CR: overwrite line
			i++
		case c == '\x1b':
			seq, n, isSGR := parseESC(data, i)
			if isSGR {
				b.cur.WriteString(seq)
			}
			if n == 0 {
				n = 1
			}
			i += n
		case c >= 0x20 || c == '\t':
			b.cur.WriteByte(c)
			i++
		default:
			i++
		}
	}
}

// parseESC parses an ANSI/VT escape sequence at data[i].
// Returns the sequence text, its byte length, and whether it is an SGR
// (colour/style: ESC [ ... m) sequence. Non-SGR sequences are stripped
// from the scrollback buffer; SGR sequences are kept.
func parseESC(data string, i int) (seq string, n int, isSGR bool) {
	start := i
	if i >= len(data) || data[i] != '\x1b' {
		return "", 0, false
	}
	i++
	if i >= len(data) {
		return data[start:i], 1, false
	}
	switch data[i] {
	case '[': // CSI
		i++
		for i < len(data) {
			b := data[i]
			if b >= 0x40 && b <= 0x7e { // final byte
				i++
				isSGR = b == 'm'
				return data[start:i], i - start, isSGR
			}
			i++
		}
		return data[start:i], i - start, false
	case ']': // OSC — skip until BEL or ESC \
		i++
		for i < len(data) {
			if data[i] == 0x07 {
				i++
				break
			}
			if data[i] == '\x1b' && i+1 < len(data) && data[i+1] == '\\' {
				i += 2
				break
			}
			i++
		}
		return data[start:i], i - start, false
	default: // 2-byte: ESC 7, ESC 8, ESC c, etc.
		i++
		return data[start:i], 2, false
	}
}

// stripAnsi removes all escape sequences for visible-width measurement.
func stripAnsi(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			_, n, _ := parseESC(s, i)
			if n == 0 {
				n = 1
			}
			i += n
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// visWidth returns the visible rune count of s, excluding ANSI sequences.
func visWidth(s string) int {
	return utf8.RuneCountInString(stripAnsi(s))
}

// truncLine truncates a line (containing ANSI colour sequences) to at most
// maxRunes visible characters, appending a reset so colours do not bleed.
func truncLine(s string, maxRunes int) string {
	var b strings.Builder
	count := 0
	i := 0
	for i < len(s) && count < maxRunes {
		if s[i] == '\x1b' {
			seq, n, _ := parseESC(s, i)
			b.WriteString(seq)
			if n == 0 {
				n = 1
			}
			i += n
			continue
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		count++
		i += sz
	}
	if count >= maxRunes {
		b.WriteString(clrReset)
	}
	return b.String()
}
