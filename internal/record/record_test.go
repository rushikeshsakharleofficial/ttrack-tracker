package record

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openDevNull opens /dev/null for reading and writing.
func openDevNull(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	return f
}

// TestRunNonInteractivePipe runs the equivalent of:
//
//	ttrack rec -q -o <tmpfile> /bin/bash -c 'echo hello-rec-test'
//
// Verifies: no error, output file exists and is non-empty, first line is a
// valid cast v2 JSON header.
func TestRunNonInteractivePipe(t *testing.T) {
	outPath := t.TempDir() + "/out.cast"

	null := openDevNull(t)
	defer null.Close()

	// Swap os.Stdin and os.Stdout before Run; restore immediately after so the
	// restore happens before any deferred t.Cleanup funcs run. This avoids a
	// race between the cleanup write and goroutines inside Run that read
	// os.Stdin (watchResize).
	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin = null
	os.Stdout = null

	err := Run([]string{"-q", "-o", outPath, "/bin/bash", "-c", "echo hello-rec-test"})

	os.Stdin = origStdin
	os.Stdout = origStdout

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	fi, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("output file is empty")
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("output file has no lines")
	}
	firstLine := sc.Text()

	var hdr struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(firstLine), &hdr); err != nil {
		t.Fatalf("first line not valid JSON: %v — line: %q", err, firstLine)
	}
	if hdr.Version != 2 {
		t.Errorf("header version = %d, want 2", hdr.Version)
	}
}

// TestRunPipeStdinExitsCleanly verifies that piping commands through stdin
// exits within a deadline and does not hang. This exercises the sync.Once /
// force-close PTY fix: closing stdin EOF → Ctrl-D write → 500ms grace →
// ptmx.Close() → SIGHUP → child exits.
func TestRunPipeStdinExitsCleanly(t *testing.T) {
	outPath := t.TempDir() + "/pipe.cast"

	// Build a stdin pipe. Write a short command then close the write end so
	// bash sees EOF; the io.Copy goroutine in Run will then trigger the PTY
	// close path.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	// Use a short, single-token command (no spaces) so bash line-editing in
	// the PTY cannot wrap it across multiple output chunks.
	if _, err := pw.WriteString("echo ok\n"); err != nil {
		t.Fatalf("write to pipe: %v", err)
	}
	pw.Close()

	null := openDevNull(t)
	defer null.Close()

	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin = pr
	os.Stdout = null

	done := make(chan error, 1)
	go func() { done <- Run([]string{"-q", "-o", outPath, "/bin/bash", "-s"}) }()

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(15 * time.Second):
		os.Stdin = origStdin
		os.Stdout = origStdout
		pr.Close()
		t.Fatal("Run did not complete within 15s — likely hung on stdin EOF")
	}

	// Restore before any assertions so the deferred null.Close is safe.
	os.Stdin = origStdin
	os.Stdout = origStdout
	pr.Close()

	// Run may return an ExitCodeError when the child is killed by SIGHUP from
	// PTY cleanup (128+1=129). Internal recorder errors are not ExitCodeErrors.
	if runErr != nil {
		if _, ok := runErr.(*ExitCodeError); !ok {
			t.Fatalf("Run returned internal recorder error: %v", runErr)
		}
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	// The output file is a cast (JSON-lines). The "ok" from "echo ok" should
	// appear somewhere in the event data.
	if !strings.Contains(string(data), "ok") {
		t.Errorf("output file does not contain expected echo output; content: %q", string(data))
	}
}

func TestExitCodeErrorNilOnNil(t *testing.T) {
	if err := exitCodeError(nil); err != nil {
		t.Fatalf("exitCodeError(nil) = %v, want nil", err)
	}
}

func TestExitCodeErrorPassesThroughNonExitError(t *testing.T) {
	sentinel := fmt.Errorf("internal recorder failure")
	if got := exitCodeError(sentinel); got != sentinel {
		t.Fatalf("non-exit error should pass through, got %v", got)
	}
}

func TestExitCodeErrorExtractsCode(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 42")
	err := cmd.Run()
	got := exitCodeError(err)
	if got == nil {
		t.Fatal("expected ExitCodeError, got nil")
	}
	ec, ok := got.(*ExitCodeError)
	if !ok {
		t.Fatalf("exitCodeError(%v) = %T, want *ExitCodeError", err, got)
	}
	if ec.Code != 42 {
		t.Errorf("ExitCodeError.Code = %d, want 42", ec.Code)
	}
}

func TestExitCodeErrorSignalIs128PlusSig(t *testing.T) {
	// SIGKILL = 9, so we expect 128+9 = 137.
	cmd := exec.Command("/bin/sh", "-c", "kill -9 $$")
	err := cmd.Run()
	got := exitCodeError(err)
	ec, ok := got.(*ExitCodeError)
	if !ok {
		t.Fatalf("expected *ExitCodeError for signal-killed process, got %T: %v", got, got)
	}
	if ec.Code != 137 {
		t.Errorf("signal-killed ExitCodeError.Code = %d, want 137 (128+SIGKILL)", ec.Code)
	}
}

func TestSSHWrapperDoesNotNestTtrackRec(t *testing.T) {
	tmp := t.TempDir()
	fakeTtrack := filepath.Join(tmp, "ttrack")
	if err := os.WriteFile(fakeTtrack, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatalf("write fake ttrack: %v", err)
	}

	wrapper := filepath.Join("..", "..", "scripts", "ttrack-ssh-wrap.sh")
	cmd := exec.Command("sh", wrapper)
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+":"+os.Getenv("PATH"),
		"SHELL=/bin/sh",
		"SSH_ORIGINAL_COMMAND=ttrack rec -q /bin/bash -s",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper returned error: %v; output: %s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "rec -q /bin/bash -s"
	if got != want {
		t.Fatalf("wrapper ttrack args = %q, want %q", got, want)
	}
}
