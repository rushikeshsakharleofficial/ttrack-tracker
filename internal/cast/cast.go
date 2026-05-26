// Package cast reads and writes the asciinema v2 cast format.
//
// A cast file is JSON-lines: the first line is a header object, and each
// subsequent line is an event array [time, type, data], e.g.
//
//	{"version":2,"width":80,"height":24,"timestamp":1700000000}
//	[0.123456, "o", "hello\r\n"]
package cast

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// Header is the first line of a cast file.
type Header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp,omitempty"`
	Command   string            `json:"command,omitempty"`
	Title     string            `json:"title,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// Event is one recorded I/O event. Type is "o" (output) or "i" (input).
type Event struct {
	Time float64
	Type string
	Data string
}

// Writer streams cast events to an underlying writer.
type Writer struct {
	w *bufio.Writer
}

// NewWriter writes the header and returns a Writer for the events.
func NewWriter(w io.Writer, h Header) (*Writer, error) {
	h.Version = 2
	bw := bufio.NewWriter(w)
	if err := json.NewEncoder(bw).Encode(h); err != nil {
		return nil, err
	}
	return &Writer{w: bw}, nil
}

// WriteOutput appends an output event at t seconds since session start.
func (c *Writer) WriteOutput(t float64, data []byte) error {
	enc, err := json.Marshal(string(data))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.w, "[%s, \"o\", %s]\n",
		strconv.FormatFloat(t, 'f', 6, 64), enc)
	return err
}

// Flush flushes buffered data to the underlying writer.
func (c *Writer) Flush() error { return c.w.Flush() }

// ReadHeader reads and parses the header line.
func ReadHeader(r *bufio.Reader) (Header, error) {
	line, err := r.ReadBytes('\n')
	if len(line) == 0 {
		if err == nil {
			err = io.EOF
		}
		return Header{}, err
	}
	var h Header
	if e := json.Unmarshal(bytes.TrimSpace(line), &h); e != nil {
		return Header{}, fmt.Errorf("invalid cast header: %w", e)
	}
	return h, nil
}

// ReadEvent reads the next event line, skipping blanks. Returns io.EOF at end.
func ReadEvent(r *bufio.Reader) (Event, error) {
	for {
		line, err := r.ReadBytes('\n')
		if len(line) == 0 {
			if err == nil {
				err = io.EOF
			}
			return Event{}, err
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			if err != nil {
				return Event{}, io.EOF
			}
			continue
		}
		var raw []json.RawMessage
		if e := json.Unmarshal(trimmed, &raw); e != nil {
			return Event{}, fmt.Errorf("bad event line: %w", e)
		}
		if len(raw) < 3 {
			return Event{}, fmt.Errorf("short event line: %s", trimmed)
		}
		var ev Event
		if e := json.Unmarshal(raw[0], &ev.Time); e != nil {
			return Event{}, fmt.Errorf("bad event time: %w", e)
		}
		_ = json.Unmarshal(raw[1], &ev.Type)
		_ = json.Unmarshal(raw[2], &ev.Data)
		return ev, nil
	}
}
