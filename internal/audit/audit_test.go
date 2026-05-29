package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ttrack/internal/config"
)

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// setupCentral points the central store at a fresh temp dir and returns it.
func setupCentral(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TTRACK_CENTRAL_DIR", dir)
	t.Setenv("TTRACK_DIR", t.TempDir())
	config.Reset()
	t.Cleanup(config.Reset)
	return dir
}

// writeCast writes a plaintext cast file for user/id with the given header
// command and output event data. No key needed (plaintext, not TTEC1).
func writeCast(t *testing.T, central, user, id, command string, outputs ...string) {
	t.Helper()
	udir := filepath.Join(central, user)
	if err := os.MkdirAll(udir, 0o700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(`{"version":2,"width":80,"height":24,"timestamp":1700000000,"command":`)
	b.WriteString(jsonStr(command))
	b.WriteString("}\n")
	for i, o := range outputs {
		b.WriteString("[")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`, "o", `)
		b.WriteString(jsonStr(o))
		b.WriteString("]\n")
	}
	if err := os.WriteFile(filepath.Join(udir, id+".cast"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, e := r.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if e != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func TestLastNLines(t *testing.T) {
	tests := []struct {
		name   string
		n      int
		events []string
		want   string
	}{
		{
			name:   "fewer than N lines",
			n:      20,
			events: []string{"line1\nline2\nline3\n"},
			want:   "line1\nline2\nline3\n",
		},
		{
			name:   "exactly trims to last N lines",
			n:      2,
			events: []string{"a\nb\nc\nd\n"},
			want:   "c\nd\n",
		},
		{
			name:   "line split across two events",
			n:      2,
			events: []string{"a\nb\npart", "ial\nc\n"},
			want:   "partial\nc\n",
		},
		{
			name:   "no trailing newline keeps partial",
			n:      2,
			events: []string{"a\nb\nc"},
			want:   "b\nc",
		},
		{
			name:   "single event single line",
			n:      5,
			events: []string{"hello world"},
			want:   "hello world",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newLineRing(tc.n)
			for _, e := range tc.events {
				r.add(e)
			}
			if got := r.String(); got != tc.want {
				t.Errorf("lastNLines = %q, want %q", got, tc.want)
			}
		})
	}
}

// Issue #10: a corrupt header must surface as an "(unreadable)" marker rather
// than rendering a blank row.
func TestLsUserUnreadableMarker(t *testing.T) {
	central := setupCentral(t)
	udir := filepath.Join(central, "alice")
	if err := os.MkdirAll(udir, 0o700); err != nil {
		t.Fatal(err)
	}
	// First line is not valid JSON -> header read fails.
	if err := os.WriteFile(filepath.Join(udir, "bad.cast"), []byte("not json at all\n[0, \"o\", \"hi\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := LsUser([]string{"alice"}); err != nil {
			t.Fatalf("LsUser: %v", err)
		}
	})
	if !strings.Contains(out, "(unreadable)") {
		t.Errorf("expected (unreadable) marker in ls-user output, got:\n%s", out)
	}
}

func TestTreeUnreadableMarker(t *testing.T) {
	central := setupCentral(t)
	udir := filepath.Join(central, "alice")
	if err := os.MkdirAll(udir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(udir, "bad.cast"), []byte("garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := Tree(nil); err != nil {
			t.Fatalf("Tree: %v", err)
		}
	})
	if !strings.Contains(out, "(unreadable)") {
		t.Errorf("expected (unreadable) marker in tree output, got:\n%s", out)
	}
}

func TestSearchUnreadableMarker(t *testing.T) {
	central := setupCentral(t)
	writeCast(t, central, "alice", "good", "echo hi", "nginx config ok\n")
	udir := filepath.Join(central, "alice")
	if err := os.WriteFile(filepath.Join(udir, "bad.cast"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := Search([]string{"--all"}); err != nil {
			t.Fatalf("Search: %v", err)
		}
	})
	if !strings.Contains(out, "(unreadable)") {
		t.Errorf("expected (unreadable) marker in search --all output, got:\n%s", out)
	}
}

// Issue #8: search must find matches and produce deterministic, user-grouped
// output that is identical across repeated runs.
func TestSearchFindsAndIsDeterministic(t *testing.T) {
	central := setupCentral(t)
	// Multiple users and sessions so the worker pool has work to reorder.
	// alice has two matching sessions to exercise intra-user ordering.
	writeCast(t, central, "alice", "a1", "echo one", "hello nginx world\n")
	writeCast(t, central, "alice", "a2", "echo two", "another nginx line\n")
	writeCast(t, central, "bob", "b1", "run nginx", "starting up\n")
	writeCast(t, central, "carol", "c1", "echo three", "nginx restart done\n")

	run := func() string {
		return captureStdout(t, func() {
			if err := Search([]string{"nginx"}); err != nil {
				t.Fatalf("Search: %v", err)
			}
		})
	}
	first := run()
	if !strings.Contains(first, "user=alice") || !strings.Contains(first, "user=bob") || !strings.Contains(first, "user=carol") {
		t.Errorf("expected matches for alice, bob, carol; got:\n%s", first)
	}
	// alice must appear before bob before carol (sorted user order preserved).
	ia := strings.Index(first, "user=alice")
	ib := strings.Index(first, "user=bob")
	ic := strings.Index(first, "user=carol")
	if !(ia < ib && ib < ic) {
		t.Errorf("user ordering not preserved: alice=%d bob=%d carol=%d\n%s", ia, ib, ic, first)
	}
	// Intra-user store order: session a1 must precede a2.
	if i1, i2 := strings.Index(first, "session=a1"), strings.Index(first, "session=a2"); i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("intra-user ordering not preserved: a1=%d a2=%d\n%s", i1, i2, first)
	}
	// Determinism: identical output across repeated runs.
	for i := 0; i < 5; i++ {
		if got := run(); got != first {
			t.Errorf("search output not deterministic on run %d:\nfirst:\n%s\ngot:\n%s", i, first, got)
		}
	}
}
