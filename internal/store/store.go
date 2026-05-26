// Package store manages where recordings live and lists them.
package store

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"ttrack/internal/cast"
)

// Dir returns the recordings directory ($TTRACK_DIR or ~/.local/share/ttrack).
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

// NewPath returns an auto-named cast path, creating the store dir if needed.
func NewPath() (string, error) {
	d := Dir()
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%d.cast", time.Now().Format("20060102T150405"), os.Getpid())
	return filepath.Join(d, name), nil
}

// List prints recorded sessions. args is unused for now.
func List(args []string) error {
	d := Dir()
	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("no recordings yet (dir: %s)\n", d)
			return nil
		}
		return err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".cast" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		fmt.Printf("no recordings in %s\n", d)
		return nil
	}

	fmt.Printf("%-26s  %-19s  %s\n", "FILE", "STARTED", "COMMAND")
	for _, name := range files {
		h, _ := readHeader(filepath.Join(d, name))
		started := "?"
		if h.Timestamp > 0 {
			started = time.Unix(h.Timestamp, 0).Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-26s  %-19s  %s\n", name, started, h.Command)
	}
	return nil
}

func readHeader(path string) (cast.Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return cast.Header{}, err
	}
	defer f.Close()
	return cast.ReadHeader(bufio.NewReader(f))
}
