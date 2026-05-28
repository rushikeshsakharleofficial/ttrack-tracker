// Package ansible records and replays Ansible playbook runs from the ttrack
// central store. The controller-side Python callback plugin writes JSON-lines
// events via `ttrack ansible-ingest`, which streams them to ttrackd over the
// same Unix socket used by `ttrack rec` (new "ANSIBLE <runid>" command).
//
// JSON-lines schema (one object per line, in order):
//
//	{"type":"run",   "id":"<ts>-<pid>","playbook":"deploy.yml","user":"alice","started":<unix>,"controller":"<host>"}
//	{"type":"play",  "name":"deploy web"}
//	{"type":"task",  "play":"…","name":"…","module":"…","host":"…","status":"ok|changed|failed|unreachable|skipped","rc":<int>,"t":<unix>,"stdout":"…","stderr":"…"}
//	{"type":"stats", "host":"…","ok":<int>,"changed":<int>,"failed":<int>,"unreachable":<int>,"skipped":<int>}
//
// stdout/stderr are omitted (or "<censored>") when no_log: true.
// They are truncated to maxOutput bytes to bound record size.
package ansible

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"ttrack/internal/config"
)

// ----------------------------------------------------------------------------
// Raw event types (JSON-lines on the wire / in storage)
// ----------------------------------------------------------------------------

type rawEvent struct {
	Type string `json:"type"`

	// "run" fields
	ID         string  `json:"id,omitempty"`
	Playbook   string  `json:"playbook,omitempty"`
	User       string  `json:"user,omitempty"`
	Started    float64 `json:"started,omitempty"`
	Controller string  `json:"controller,omitempty"`

	// "play" fields
	Name string `json:"name,omitempty"`

	// "task" fields
	Play   string  `json:"play,omitempty"`
	Module string  `json:"module,omitempty"`
	Host   string  `json:"host,omitempty"`
	Status string  `json:"status,omitempty"`
	RC     int     `json:"rc,omitempty"`
	T      float64 `json:"t,omitempty"`
	Stdout string  `json:"stdout,omitempty"`
	Stderr string  `json:"stderr,omitempty"`

	// "stats" fields
	OK          int `json:"ok,omitempty"`
	Changed     int `json:"changed,omitempty"`
	Failed      int `json:"failed,omitempty"`
	Unreachable int `json:"unreachable,omitempty"`
	Skipped     int `json:"skipped,omitempty"`
}

// ----------------------------------------------------------------------------
// Rich model types
// ----------------------------------------------------------------------------

// Task is a single Ansible task execution on one host.
type Task struct {
	Play   string
	Name   string
	Module string
	Host   string
	Status string // ok, changed, failed, unreachable, skipped
	RC     int
	T      float64 // Unix timestamp
	Stdout string
	Stderr string
}

// HostStats is the recap for one host at the end of a playbook run.
type HostStats struct {
	Host        string
	OK          int
	Changed     int
	Failed      int
	Unreachable int
	Skipped     int
}

// Run is a complete Ansible playbook execution.
type Run struct {
	ID         string
	Playbook   string
	User       string
	Started    time.Time
	Controller string

	Plays []string // ordered play names
	Tasks []Task

	// Stats indexed by host name.
	Stats map[string]HostStats

	// Derived totals (computed by ParseRun).
	TotalOK          int
	TotalChanged     int
	TotalFailed      int
	TotalUnreachable int
	TotalSkipped     int

	// Hosts that appear in any task or stats (ordered).
	Hosts []string
}

// Duration returns wallclock duration from Run.Started to the last task
// timestamp (or zero when no tasks).
func (r *Run) Duration() time.Duration {
	if len(r.Tasks) == 0 {
		return 0
	}
	last := r.Tasks[len(r.Tasks)-1].T
	return time.Duration((last-float64(r.Started.Unix()))*1e9) * time.Nanosecond
}

// ----------------------------------------------------------------------------
// ParseRun
// ----------------------------------------------------------------------------

// ParseRun decodes a JSON-lines stream into a Run. Lines that do not parse as
// valid JSON objects are silently skipped (robustness). Returns an error only
// when the stream cannot be read at all.
func ParseRun(r io.Reader) (*Run, error) {
	run := &Run{Stats: map[string]HostStats{}}
	hostSet := map[string]struct{}{}
	playSet := map[string]struct{}{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 256*1024), 256*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		var ev rawEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "run":
			run.ID = ev.ID
			run.Playbook = ev.Playbook
			run.User = ev.User
			run.Controller = ev.Controller
			if ev.Started > 0 {
				run.Started = time.Unix(int64(ev.Started), 0)
			}

		case "play":
			if ev.Name != "" {
				if _, seen := playSet[ev.Name]; !seen {
					playSet[ev.Name] = struct{}{}
					run.Plays = append(run.Plays, ev.Name)
				}
			}

		case "task":
			t := Task{
				Play:   ev.Play,
				Name:   ev.Name,
				Module: ev.Module,
				Host:   ev.Host,
				Status: ev.Status,
				RC:     ev.RC,
				T:      ev.T,
				Stdout: truncate(ev.Stdout),
				Stderr: truncate(ev.Stderr),
			}
			run.Tasks = append(run.Tasks, t)
			if ev.Host != "" {
				hostSet[ev.Host] = struct{}{}
			}

		case "stats":
			s := HostStats{
				Host:        ev.Host,
				OK:          ev.OK,
				Changed:     ev.Changed,
				Failed:      ev.Failed,
				Unreachable: ev.Unreachable,
				Skipped:     ev.Skipped,
			}
			run.Stats[ev.Host] = s
			run.TotalOK += ev.OK
			run.TotalChanged += ev.Changed
			run.TotalFailed += ev.Failed
			run.TotalUnreachable += ev.Unreachable
			run.TotalSkipped += ev.Skipped
			if ev.Host != "" {
				hostSet[ev.Host] = struct{}{}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("ansible: parse: %w", err)
	}

	// Sort hosts for deterministic output.
	for h := range hostSet {
		run.Hosts = append(run.Hosts, h)
	}
	sort.Strings(run.Hosts)
	return run, nil
}

// truncate caps s to config.AnsibleOutputCap bytes (UTF-8 safe: trims at a rune boundary).
func truncate(s string) string {
	maxOutput := config.Load().AnsibleOutputCap
	if len(s) <= maxOutput {
		return s
	}
	// Trim at a safe UTF-8 boundary.
	b := []byte(s[:maxOutput])
	// Walk back if we sliced in the middle of a multi-byte rune.
	for len(b) > 0 && b[len(b)-1]&0xC0 == 0x80 {
		b = b[:len(b)-1]
	}
	return string(b) + "\n[... truncated]"
}
