package cast

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestRoundTrip writes a header + several events and reads them back,
// verifying that all fields and special characters survive the round-trip.
func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	h := Header{
		Width:     80,
		Height:    24,
		Timestamp: 1700000000,
		Command:   "bash",
		Title:     "my session",
		Env:       map[string]string{"TERM": "xterm"},
	}

	w, err := NewWriter(&buf, h)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	type eventIn struct {
		t    float64
		data []byte
	}
	events := []eventIn{
		{0.0, []byte("hello\r\n")},
		{1.5, []byte("tab\tend")},
		{2.25, []byte(`say "hi"`)},
		{3.0, []byte("line1\nline2\n")},
		{100.123456, []byte("end")},
	}

	for _, e := range events {
		if err := w.WriteOutput(e.t, e.data); err != nil {
			t.Fatalf("WriteOutput(%v): %v", e.t, err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	r := bufio.NewReader(bytes.NewReader(buf.Bytes()))

	got, err := ReadHeader(r)
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version: got %d, want 2", got.Version)
	}
	if got.Width != 80 {
		t.Errorf("Width: got %d, want 80", got.Width)
	}
	if got.Height != 24 {
		t.Errorf("Height: got %d, want 24", got.Height)
	}
	if got.Timestamp != 1700000000 {
		t.Errorf("Timestamp: got %d, want 1700000000", got.Timestamp)
	}
	if got.Command != "bash" {
		t.Errorf("Command: got %q, want %q", got.Command, "bash")
	}
	if got.Title != "my session" {
		t.Errorf("Title: got %q, want %q", got.Title, "my session")
	}
	if got.Env["TERM"] != "xterm" {
		t.Errorf("Env[TERM]: got %q, want %q", got.Env["TERM"], "xterm")
	}

	for i, e := range events {
		ev, err := ReadEvent(r)
		if err != nil {
			t.Fatalf("ReadEvent[%d]: %v", i, err)
		}
		if ev.Time != e.t {
			t.Errorf("event[%d].Time: got %v, want %v", i, ev.Time, e.t)
		}
		if ev.Type != "o" {
			t.Errorf("event[%d].Type: got %q, want %q", i, ev.Type, "o")
		}
		if ev.Data != string(e.data) {
			t.Errorf("event[%d].Data: got %q, want %q", i, ev.Data, string(e.data))
		}
	}
}

// TestSplitMultibyteRune verifies that a UTF-8 rune split across two writes
// (as PTY reads can do) survives the round-trip intact rather than being
// corrupted to U+FFFD. Reproduces the "gibberish in editor replay" bug.
func TestSplitMultibyteRune(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Header{Width: 80, Height: 24})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	// "│" (U+2502, box-drawing) is 3 bytes: 0xE2 0x94 0x82. Split after byte 1.
	full := []byte("ab│cd")
	idx := bytes.IndexByte(full, 0xE2) + 1 // split mid-rune
	if err := w.WriteOutput(0.0, full[:idx]); err != nil {
		t.Fatalf("WriteOutput(part1): %v", err)
	}
	if err := w.WriteOutput(0.5, full[idx:]); err != nil {
		t.Fatalf("WriteOutput(part2): %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	if _, err := ReadHeader(r); err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	var got string
	for {
		ev, err := ReadEvent(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ReadEvent: %v", err)
		}
		got += ev.Data
	}
	if got != string(full) {
		t.Errorf("reassembled: got %q, want %q", got, string(full))
	}
	if strings.ContainsRune(got, '�') {
		t.Errorf("output contains U+FFFD replacement char: %q", got)
	}
}

// TestReadEventEOF asserts that reading past the last event returns io.EOF.
func TestReadEventEOF(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Header{Width: 80, Height: 24})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.WriteOutput(0.0, []byte("x")); err != nil {
		t.Fatalf("WriteOutput: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	r := bufio.NewReader(bytes.NewReader(buf.Bytes()))

	if _, err := ReadHeader(r); err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if _, err := ReadEvent(r); err != nil {
		t.Fatalf("ReadEvent (first): %v", err)
	}
	_, err = ReadEvent(r)
	if err != io.EOF {
		t.Errorf("ReadEvent past end: got %v, want io.EOF", err)
	}
}

// TestBlankLinesSkipped verifies that blank lines between events are ignored.
func TestBlankLinesSkipped(t *testing.T) {
	// Build a cast body manually: header + event + blank + event.
	body := `{"version":2,"width":80,"height":24}
[0.100000, "o", "first"]

[1.200000, "o", "second"]
`
	r := bufio.NewReader(strings.NewReader(body))

	if _, err := ReadHeader(r); err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}

	e1, err := ReadEvent(r)
	if err != nil {
		t.Fatalf("ReadEvent[0]: %v", err)
	}
	if e1.Data != "first" {
		t.Errorf("event[0].Data: got %q, want %q", e1.Data, "first")
	}

	e2, err := ReadEvent(r)
	if err != nil {
		t.Fatalf("ReadEvent[1]: %v", err)
	}
	if e2.Data != "second" {
		t.Errorf("event[1].Data: got %q, want %q", e2.Data, "second")
	}

	_, err = ReadEvent(r)
	if err != io.EOF {
		t.Errorf("ReadEvent past end: got %v, want io.EOF", err)
	}
}

// TestReadHeaderInvalid asserts that a non-JSON first line produces an error.
func TestReadHeaderInvalid(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"plain text", "not json\n"},
		{"empty object fragment", "{\n"},
		{"bare number", "42\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tc.input))
			_, err := ReadHeader(r)
			if err == nil {
				t.Errorf("ReadHeader(%q): expected error, got nil", tc.input)
			}
		})
	}
}
