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

// runeWidth returns the number of terminal columns occupied by r.
// Most runes are 1 column; CJK ideographs, fullwidth forms, and most
// emoji are 2 columns (East Asian Width W or F).
func runeWidth(r rune) int {
	if r < 0x1100 {
		return 1
	}
	switch {
	case r >= 0x1100 && r <= 0x115F:
		return 2 // Hangul Jamo
	case r == 0x2329 || r == 0x232A:
		return 2 // Angle brackets
	case r >= 0x2E80 && r <= 0x303E:
		return 2 // CJK Radicals .. CJK Symbols
	case r >= 0x3041 && r <= 0x33FF:
		return 2 // Hiragana..CJK Compatibility
	case r >= 0x3400 && r <= 0x4DBF:
		return 2 // CJK Extension A
	case r >= 0x4E00 && r <= 0xA4CF:
		return 2 // CJK Unified + Yi
	case r >= 0xA960 && r <= 0xA97F:
		return 2 // Hangul Jamo Extended-A
	case r >= 0xAC00 && r <= 0xD7AF:
		return 2 // Hangul Syllables
	case r >= 0xF900 && r <= 0xFAFF:
		return 2 // CJK Compatibility Ideographs
	case r >= 0xFE10 && r <= 0xFE19:
		return 2 // Vertical Forms
	case r >= 0xFE30 && r <= 0xFE4F:
		return 2 // CJK Compatibility Forms
	case r >= 0xFF01 && r <= 0xFF60:
		return 2 // Fullwidth Forms
	case r >= 0xFFE0 && r <= 0xFFE6:
		return 2 // Fullwidth Signs
	case r >= 0x1B000 && r <= 0x1B0FF:
		return 2 // Kana Supplement
	case r >= 0x1F004 && r <= 0x1FFFD:
		return 2 // Emoji and supplemental symbols
	case r >= 0x20000 && r <= 0x2FFFD:
		return 2 // CJK Extension B-F
	case r >= 0x30000 && r <= 0x3FFFD:
		return 2 // CJK Extension G+
	}
	return 1
}

// visWidth returns the visible terminal column width of s, excluding ANSI sequences.
func visWidth(s string) int {
	plain := stripAnsi(s)
	w := 0
	for _, r := range plain {
		w += runeWidth(r)
	}
	return w
}

// truncLine truncates a line (containing ANSI colour sequences) to at most
// maxCols visible terminal columns, appending a reset so colours do not bleed.
func truncLine(s string, maxCols int) string {
	var b strings.Builder
	cols := 0
	i := 0
	for i < len(s) {
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
		rw := runeWidth(r)
		if cols+rw > maxCols {
			break // would overflow — stop here
		}
		b.WriteRune(r)
		cols += rw
		i += sz
	}
	if cols >= maxCols {
		b.WriteString(clrReset)
	}
	return b.String()
}
