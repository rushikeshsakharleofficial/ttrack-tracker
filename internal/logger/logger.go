// Package logger provides leveled logging for ttrack and ttrackd.
//
// Levels:
//
//	0 = OFF   — no output
//	1 = ERROR — fatal errors, write failures
//	2 = WARN  — retries, recoverable errors, fallback paths
//	3 = INFO  — startup, session open/close  (default)
//	4 = DEBUG — frame details, config loading, connection flow
//	5 = TRACE — every read/write byte count, buffer operations
//
// Under systemd (JOURNAL_STREAM set) timestamps are stripped — journald adds them.
// Standalone: standard log timestamps are used.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// Level is a logging verbosity level (0–5).
type Level int32

const (
	LevelOff   Level = 0
	LevelError Level = 1
	LevelWarn  Level = 2
	LevelInfo  Level = 3
	LevelDebug Level = 4
	LevelTrace Level = 5
)

var current atomic.Int32

// baseWriter is the log.Writer() captured on the first TeeToFile call, before
// any file tee is installed. Subsequent Reopen calls always tee from this base
// so MultiWriters never stack. nil until the first TeeToFile call.
var baseWriter io.Writer

// currentFile is the currently open log file (nil when file logging is off).
var currentFile *os.File

// logMu guards baseWriter, currentFile, and log.SetOutput calls.
var logMu sync.Mutex

func init() {
	current.Store(int32(LevelInfo))
	// Under systemd journald, timestamps are added automatically.
	if os.Getenv("JOURNAL_STREAM") != "" || os.Getenv("INVOCATION_ID") != "" {
		log.SetFlags(0)
	}
}

// Set changes the active log level. Thread-safe.
func Set(l Level) { current.Store(int32(l)) }

// Get returns the current log level.
func Get() Level { return Level(current.Load()) }

// TeeToFile sends future log output to the base writer and path.
// Parent directories are created when missing. The returned function disables
// file logging and closes the file.
func TeeToFile(path string) (func() error, error) {
	if path == "" {
		return func() error { return nil }, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0o640); err != nil {
		_ = f.Close()
		return nil, err
	}
	logMu.Lock()
	if baseWriter == nil {
		baseWriter = log.Writer() // capture once, before any file tee
	}
	log.SetOutput(io.MultiWriter(baseWriter, f))
	oldFile := currentFile
	currentFile = f
	logMu.Unlock()
	if oldFile != nil {
		_ = oldFile.Close()
	}
	return func() error {
		logMu.Lock()
		if baseWriter != nil {
			log.SetOutput(baseWriter)
		}
		cur := currentFile
		currentFile = nil
		logMu.Unlock()
		if cur != nil {
			return cur.Close()
		}
		return nil
	}, nil
}

// Reopen closes the current log file and reopens it at path. Call on SIGHUP
// so logrotate can rename the old file without losing future log lines.
// An empty path disables file logging.
func Reopen(path string) error {
	if path == "" {
		logMu.Lock()
		log.SetOutput(baseWriter)
		old := currentFile
		currentFile = nil
		logMu.Unlock()
		if old != nil {
			return old.Close()
		}
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o640); err != nil {
		_ = f.Close()
		return err
	}
	logMu.Lock()
	log.SetOutput(io.MultiWriter(baseWriter, f))
	old := currentFile
	currentFile = f
	logMu.Unlock()
	if old != nil {
		return old.Close()
	}
	return nil
}

// Errorf logs at level ERROR (1).
func Errorf(format string, args ...any) { emit(LevelError, "ERROR", format, args...) }

// Warnf logs at level WARN (2).
func Warnf(format string, args ...any) { emit(LevelWarn, "WARN", format, args...) }

// Infof logs at level INFO (3).
func Infof(format string, args ...any) { emit(LevelInfo, "INFO", format, args...) }

// Debugf logs at level DEBUG (4).
func Debugf(format string, args ...any) { emit(LevelDebug, "DEBUG", format, args...) }

// Tracef logs at level TRACE (5).
func Tracef(format string, args ...any) { emit(LevelTrace, "TRACE", format, args...) }

func emit(level Level, tag, format string, args ...any) {
	l := Level(current.Load())
	if l == LevelOff || level > l {
		return
	}
	msg := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", tag, msg)
}
