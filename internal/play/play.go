// Package play replays a recorded cast file to the terminal.
package play

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"ttrack/internal/cast"
	"ttrack/internal/store"
)

// Run replays a session. args is the play subcommand's argv (after "play").
func Run(args []string) error {
	fs := flag.NewFlagSet("play", flag.ContinueOnError)
	speed := fs.Float64("speed", 1.0, "playback speed multiplier")
	maxIdle := fs.Float64("idle", 2.0, "cap idle gaps to N seconds (0 = no cap)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ttrack play [--speed N] [--idle N] <file>")
	}
	if *speed <= 0 {
		return fmt.Errorf("--speed must be > 0")
	}

	// Resolve the argument: try it as given, then fall back to the store dir
	// so a bare filename from `ttrack ls` works from any directory.
	name := fs.Arg(0)
	path := name
	if _, statErr := os.Stat(path); statErr != nil && !filepath.IsAbs(name) {
		if alt := filepath.Join(store.Dir(), name); fileExists(alt) {
			path = alt
		}
	}
	return PlayFile(path, *speed, *maxIdle)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// PlayFile replays the cast at path with the given speed and idle cap,
// transparently decrypting encrypted recordings.
func PlayFile(path string, speed, maxIdle float64) error {
	if speed <= 0 {
		speed = 1.0
	}
	rc, err := store.OpenCast(path)
	if err != nil {
		return err
	}
	defer rc.Close()
	return play(rc, speed, maxIdle)
}

func play(rc io.Reader, speed, maxIdle float64) error {
	r := bufio.NewReader(rc)
	if _, err := cast.ReadHeader(r); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "--- ttrack replay start ---")
	defer fmt.Fprintln(os.Stderr, "\r\n--- ttrack replay end ---")

	var last float64
	for {
		ev, err := cast.ReadEvent(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		gap := ev.Time - last
		last = ev.Time
		if maxIdle > 0 && gap > maxIdle {
			gap = maxIdle
		}
		if gap > 0 {
			time.Sleep(time.Duration(gap / speed * float64(time.Second)))
		}
		if ev.Type == "o" {
			_, _ = io.WriteString(os.Stdout, ev.Data)
		}
	}
	return nil
}
