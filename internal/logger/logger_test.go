package logger

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeeToFileCreatesParentAndWritesLog(t *testing.T) {
	var stderr bytes.Buffer
	log.SetOutput(&stderr)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})

	path := filepath.Join(t.TempDir(), "var", "log", "ttrack", "ttrack.log")
	closeLog, err := TeeToFile(path)
	if err != nil {
		t.Fatalf("TeeToFile() error = %v", err)
	}

	Infof("file logging works")

	if err := closeLog(); err != nil {
		t.Fatalf("close log: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(b), "[INFO] file logging works") {
		t.Fatalf("log file missing message: %q", string(b))
	}
	if !strings.Contains(stderr.String(), "[INFO] file logging works") {
		t.Fatalf("stderr missing message: %q", stderr.String())
	}
}

func TestTeeToFileRepairsExistingDirectoryAndFileModes(t *testing.T) {
	var stderr bytes.Buffer
	log.SetOutput(&stderr)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})

	dir := filepath.Join(t.TempDir(), "ttrack")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ttrack.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o666); err != nil {
		t.Fatal(err)
	}

	closeLog, err := TeeToFile(path)
	if err != nil {
		t.Fatalf("TeeToFile() error = %v", err)
	}
	if err := closeLog(); err != nil {
		t.Fatalf("close log: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o750 {
		t.Fatalf("directory mode = %o, want 750", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o640 {
		t.Fatalf("file mode = %o, want 640", got)
	}
}
