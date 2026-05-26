// Package store manages where recordings live and how they are listed.
//
// Two locations exist:
//   - user-local: $TTRACK_DIR or ~/.local/share/ttrack (fallback when the
//     daemon is down; later swept into the central store).
//   - central:    $TTRACK_CENTRAL_DIR or /var/lib/ttrack, root:root 0700,
//     per-user subdirs, files 0600 — only root can read these.
package store

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ttrack/internal/cast"
	"ttrack/internal/crypto"
)

// Dir returns the user-local recordings directory.
func Dir() string {
	if d := os.Getenv("TTRACK_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ttrack"
	}
	return filepath.Join(home, ".local", "share", "ttrack")
}

// CentralDir returns the central root-only recordings directory.
func CentralDir() string {
	if d := os.Getenv("TTRACK_CENTRAL_DIR"); d != "" {
		return d
	}
	return "/var/lib/ttrack"
}

// KeyPath returns the at-rest encryption key path (root-only).
func KeyPath() string {
	return filepath.Join(CentralDir(), ".ttrack.key")
}

type castReadCloser struct {
	io.Reader
	f *os.File
}

func (c *castReadCloser) Close() error { return c.f.Close() }

// OpenCast opens a cast file for reading, transparently decrypting it if it is
// an encrypted (magic-prefixed) file. Reads follow the file to its end,
// including data appended after opening (used by live tail).
func OpenCast(path string) (io.ReadCloser, error) {
	return openCast(path, false)
}

// OpenCastSnapshot is like OpenCast but bounded to the file's size at open
// time, so replaying an in-progress recording stops at the point playback
// began instead of following (and never finishing) a still-growing session.
func OpenCastSnapshot(path string) (io.ReadCloser, error) {
	return openCast(path, true)
}

func openCast(path string, snapshot bool) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var size int64 = -1
	if snapshot {
		if fi, serr := f.Stat(); serr == nil {
			size = fi.Size()
		}
	}
	magic := make([]byte, len(crypto.Magic))
	n, _ := io.ReadFull(f, magic)
	if n == len(crypto.Magic) && string(magic) == crypto.Magic {
		key, kerr := os.ReadFile(KeyPath())
		if kerr != nil {
			f.Close()
			return nil, fmt.Errorf("cannot read decryption key %s: %w", KeyPath(), kerr)
		}
		var src io.Reader = f
		if size >= 0 {
			src = io.LimitReader(f, size-int64(len(crypto.Magic)))
		}
		dr, derr := crypto.NewReader(src, key)
		if derr != nil {
			f.Close()
			return nil, derr
		}
		return &castReadCloser{Reader: dr, f: f}, nil
	}
	// Plaintext: rewind and hand back the raw file (bounded if snapshot).
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	if size >= 0 {
		return &castReadCloser{Reader: io.LimitReader(f, size), f: f}, nil
	}
	return f, nil
}

// NewPath returns an auto-named cast path in the user-local dir, creating it.
func NewPath() (string, error) {
	d := Dir()
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(d, NewName()), nil
}

// NewName returns an auto-generated cast filename: <timestamp>-<pid>.cast.
func NewName() string {
	return fmt.Sprintf("%s-%d.cast", time.Now().Format("20060102T150405"), os.Getpid())
}

// List prints user-local recordings (the personal view).
func List(args []string) error {
	d := Dir()
	files, err := castsIn(d)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("no recordings yet (dir: %s)\n", d)
			return nil
		}
		return err
	}
	if len(files) == 0 {
		fmt.Printf("no recordings in %s\n", d)
		return nil
	}
	fmt.Printf("%-7s  %-26s  %-19s  %s\n", "STATUS", "FILE", "STARTED", "COMMAND")
	for _, name := range files {
		h, _ := readHeader(filepath.Join(d, name))
		status := "SAVED"
		if isActive(name) {
			status = "ACTIVE"
		}
		fmt.Printf("%-7s  %-26s  %-19s  %s\n", status, name, started(h), h.Command)
	}
	return nil
}

// Users lists usernames that have a directory in the central store.
func Users() ([]string, error) {
	entries, err := os.ReadDir(CentralDir())
	if err != nil {
		return nil, err
	}
	var users []string
	for _, e := range entries {
		if e.IsDir() {
			users = append(users, e.Name())
		}
	}
	sort.Strings(users)
	return users, nil
}

// UserSessions lists the cast filenames for a user in the central store.
func UserSessions(user string) ([]string, error) {
	return castsIn(filepath.Join(CentralDir(), user))
}

// FindCentral locates a session by id (filename or its stem) across all users
// in the central store. Returns the full path and the owning user.
func FindCentral(id string) (path, user string, err error) {
	id = strings.TrimSuffix(id, ".cast")
	users, err := Users()
	if err != nil {
		return "", "", err
	}
	for _, u := range users {
		names, _ := UserSessions(u)
		for _, n := range names {
			if strings.TrimSuffix(n, ".cast") == id {
				return filepath.Join(CentralDir(), u, n), u, nil
			}
		}
	}
	return "", "", fmt.Errorf("session %q not found in %s", id, CentralDir())
}

func castsIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".cast" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func started(h cast.Header) string {
	if h.Timestamp > 0 {
		return time.Unix(h.Timestamp, 0).Format("2006-01-02 15:04:05")
	}
	return "?"
}

func readHeader(path string) (cast.Header, error) {
	rc, err := OpenCast(path)
	if err != nil {
		return cast.Header{}, err
	}
	defer rc.Close()
	return cast.ReadHeader(bufio.NewReader(rc))
}

// Header reads and returns a cast file's header (exported for CLI use).
func Header(path string) (cast.Header, error) { return readHeader(path) }

// Started formats a header's start time (exported for CLI use).
func Started(h cast.Header) string { return started(h) }

// isActive reports whether the recorder that created an auto-named file
// (<timestamp>-<pid>.cast) is still running. Linux-specific via /proc.
func isActive(name string) bool {
	base := strings.TrimSuffix(filepath.Base(name), ".cast")
	i := strings.LastIndex(base, "-")
	if i < 0 {
		return false
	}
	pid, err := strconv.Atoi(base[i+1:])
	if err != nil {
		return false
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(comm)) == "ttrack"
}

// IsActive is the exported form of isActive for CLI use.
func IsActive(name string) bool { return isActive(name) }
