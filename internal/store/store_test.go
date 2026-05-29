package store

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"ttrack/internal/cast"
	"ttrack/internal/config"
	"ttrack/internal/crypto"
)

// setupTempEnv points TTRACK_DIR, TTRACK_CENTRAL_DIR and TTRACK_KEY_FILE at
// temp paths and resets the config singleton so store picks them up.
func setupTempEnv(t *testing.T) (dir, central, keyFile string) {
	t.Helper()
	dir = t.TempDir()
	central = t.TempDir()
	keyFile = filepath.Join(t.TempDir(), "ttrack.key")
	t.Setenv("TTRACK_DIR", dir)
	t.Setenv("TTRACK_CENTRAL_DIR", central)
	t.Setenv("TTRACK_KEY_FILE", keyFile)
	config.Reset()
	t.Cleanup(config.Reset)
	return dir, central, keyFile
}

func TestNewNameFormat(t *testing.T) {
	name := NewName()
	re := regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}-\d+\.cast$`)
	if !re.MatchString(name) {
		t.Fatalf("NewName() = %q, does not match <timestamp>-<pid>.cast", name)
	}
}

func TestParseNameTimeRoundTrip(t *testing.T) {
	now := time.Now()
	stamp := now.Format("20060102T150405.000000000")
	got, err := parseNameTime(stamp)
	if err != nil {
		t.Fatalf("parseNameTime(%q) error: %v", stamp, err)
	}
	// Round-trip the formatted stamp; compare re-formatted strings to avoid
	// sub-nanosecond / monotonic-clock differences.
	if got.Format("20060102T150405.000000000") != stamp {
		t.Fatalf("round-trip mismatch: got %q want %q",
			got.Format("20060102T150405.000000000"), stamp)
	}
}

func TestIsActiveFalseCases(t *testing.T) {
	// Malformed name: no "-pid" suffix.
	if isActive("not-a-valid-name") {
		t.Errorf("isActive on malformed name = true, want false")
	}
	if IsActive("garbage") {
		t.Errorf("IsActive on malformed name = true, want false")
	}
	// Well-formed name but an implausible / dead pid that is not a running
	// ttrack process.
	stamp := time.Now().Format("20060102T150405.000000000")
	deadPid := 2147483646 // near max int32, not a live ttrack process
	name := stamp + "-" + strconv.Itoa(deadPid) + ".cast"
	if isActive(name) {
		t.Errorf("isActive on dead-pid name = true, want false")
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		secs float64
		want string
	}{
		{0, "0s"},
		{-5, "0s"},
		{5, "5s"},
		{65, "1m05s"},
		{3661, "1h01m01s"},
	}
	for _, c := range cases {
		if got := humanDuration(c.secs); got != c.want {
			t.Errorf("humanDuration(%v) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestTrunc(t *testing.T) {
	if got := trunc("short", 20); got != "short" {
		t.Errorf("trunc no-op = %q, want %q", got, "short")
	}
	if got := trunc("hello world", 5); got != "hell…" {
		t.Errorf("trunc long = %q, want %q", got, "hell…")
	}
	// n<=3 edge: byte-slice without ellipsis.
	if got := trunc("hello", 3); got != "hel" {
		t.Errorf("trunc n<=3 = %q, want %q", got, "hel")
	}
	if got := trunc("hello", 0); got != "" {
		t.Errorf("trunc n=0 = %q, want %q", got, "")
	}
}

// buildCast renders a small plaintext cast payload via the cast package.
func buildCast(t *testing.T, cmd, out string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := cast.NewWriter(&buf, cast.Header{Width: 80, Height: 24, Timestamp: 1700000000, Command: cmd})
	if err != nil {
		t.Fatalf("cast.NewWriter: %v", err)
	}
	if err := w.WriteOutput(0.1, []byte(out)); err != nil {
		t.Fatalf("WriteOutput: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("cast Close: %v", err)
	}
	return buf.Bytes()
}

func TestOpenCastPlaintext(t *testing.T) {
	dir, _, _ := setupTempEnv(t)
	plain := buildCast(t, "echo hi", "hi\r\n")
	p := filepath.Join(dir, "plain.cast")
	if err := os.WriteFile(p, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	rc, err := OpenCast(p)
	if err != nil {
		t.Fatalf("OpenCast plaintext: %v", err)
	}
	defer rc.Close()
	got, err := readAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("OpenCast plaintext = %q, want %q", got, plain)
	}
}

func TestOpenCastEncrypted(t *testing.T) {
	dir, _, keyFile := setupTempEnv(t)
	key := make([]byte, crypto.KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	if err := os.WriteFile(keyFile, key, 0o600); err != nil {
		t.Fatal(err)
	}
	plain := buildCast(t, "secret-cmd", "top secret\r\n")

	p := filepath.Join(dir, "enc.cast")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w, err := crypto.NewWriter(f, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	rc, err := OpenCast(p)
	if err != nil {
		t.Fatalf("OpenCast encrypted: %v", err)
	}
	defer rc.Close()
	got, err := readAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("OpenCast decrypted = %q, want %q", got, plain)
	}
}

func TestFindCentral(t *testing.T) {
	_, central, _ := setupTempEnv(t)
	user := "alice"
	id := "20240101T120000.000000000-123"
	userDir := filepath.Join(central, user)
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(userDir, id+".cast")
	if err := os.WriteFile(want, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, lookup := range []string{id, id + ".cast"} {
		path, gotUser, err := FindCentral(lookup)
		if err != nil {
			t.Fatalf("FindCentral(%q) error: %v", lookup, err)
		}
		if path != want {
			t.Errorf("FindCentral(%q) path = %q, want %q", lookup, path, want)
		}
		if gotUser != user {
			t.Errorf("FindCentral(%q) user = %q, want %q", lookup, gotUser, user)
		}
	}

	if _, _, err := FindCentral("does-not-exist"); err == nil {
		t.Errorf("FindCentral(missing) error = nil, want non-nil")
	}
}

func TestIsAnsibleRun(t *testing.T) {
	_, central, _ := setupTempEnv(t)
	user := "bob"
	runid := "20240101T120000-run1"
	ansibleDir := filepath.Join(central, user, "ansible")
	if err := os.MkdirAll(ansibleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ansibleDir, runid+".ajsonl"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !IsAnsibleRun(runid) {
		t.Errorf("IsAnsibleRun(%q) = false, want true", runid)
	}
	if IsAnsibleRun("no-such-run") {
		t.Errorf("IsAnsibleRun(missing) = true, want false")
	}
}

func TestListUnreadableHeader(t *testing.T) {
	dir, _, _ := setupTempEnv(t)
	// A .cast file with garbage (no valid header) should not crash List and
	// should be surfaced rather than rendered with blank fields.
	bad := filepath.Join(dir, "20240101T120000.000000000-999999999.cast")
	if err := os.WriteFile(bad, []byte("not json at all\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := List(nil); err != nil {
			t.Fatalf("List error: %v", err)
		}
	})
	if !bytes.Contains([]byte(out), []byte("(unreadable)")) {
		t.Fatalf("List did not surface unreadable recording; output:\n%s", out)
	}
}

// --- helpers ---

func readAll(rc interface{ Read([]byte) (int, error) }) ([]byte, error) {
	br := bufio.NewReader(rc)
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(br); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r)
		done <- b.Bytes()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return string(<-done)
}
