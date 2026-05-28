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

// closeCurrent holds the closer for the active log file so Reopen can swap it.
var closeCurrent func() error = func() error { return nil }

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

// TeeToFile sends future log output to the existing logger output and path.
// Parent directories are created when missing. The returned function restores
// the previous output and closes the file.
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
	prev := log.Writer()
	log.SetOutput(io.MultiWriter(prev, f))
	restore := func() error {
		log.SetOutput(prev)
		return f.Close()
	}
	closeCurrent = restore // save so Reopen can swap
	return restore, nil
}

// Reopen closes and reopens the log file at path. Call on SIGHUP so logrotate
// can rename the old file without losing future log lines.
func Reopen(path string) error {
	if path == "" {
		return nil
	}
	old := closeCurrent
	_, err := TeeToFile(path)
	if err != nil {
		return err
	}
	return old() // close old fd
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
