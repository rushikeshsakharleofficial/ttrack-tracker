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
