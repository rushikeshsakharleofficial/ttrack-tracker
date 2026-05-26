// Package play replays a recorded cast file to the terminal.
package play

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"ttrack/internal/cast"
	"ttrack/internal/store"
)

// Run replays a session. args is the play subcommand's argv (after "play").
func Run(args []string) error {
	fs := flag.NewFlagSet("play", flag.ContinueOnError)
	speed := fs.Float64("speed", 1.0, "playback speed multiplier")
	maxIdle := fs.Float64("idle", 0, "cap idle gaps to N seconds (default 0 = exact original timing)")
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
		} else if cp, _, err := store.FindCentral(name); err == nil {
			path = cp
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
	// Snapshot at open time: replaying an in-progress recording stops at the
	// point playback began rather than following the still-growing file.
	rc, err := store.OpenCastSnapshot(path)
	if err != nil {
		return err
	}
	defer rc.Close()

	// A recorded TUI (vim, etc.) emits terminal queries (OSC color, Device
	// Attributes). Replaying them makes the real terminal answer on stdin.
	// With echo on, the line discipline prints those answers as garbage during
	// replay; leftover bytes then run as shell commands after we exit. Disable
	// echo/canonical for the duration and drain stdin at the end.
	restore := beginReplayInput()
	err = play(rc, speed, maxIdle)
	drainTerminalInput()
	if restore != nil {
		restore()
	}
	return err
}

// beginReplayInput puts stdin in a no-echo, non-canonical mode (keeping signal
// keys like Ctrl-C working) so terminal query responses aren't echoed to the
// screen during replay. Returns a restore func, or nil if stdin isn't a tty.
func beginReplayInput() func() {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil
	}
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil
	}
	raw := *old
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return nil
	}
	restore := func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, old) }
	// Restore on Ctrl-C so an aborted replay doesn't leave the shell broken.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func() {
		if _, ok := <-sigc; ok {
			restore()
			os.Exit(130)
		}
	}()
	return func() {
		signal.Stop(sigc)
		close(sigc)
		restore()
	}
}

// drainTerminalInput discards bytes the terminal sent in reply to query
// sequences echoed during replay, so they don't leak onto the shell prompt.
// Assumes stdin is already in non-canonical mode (see beginReplayInput).
func drainTerminalInput() {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		return
	}
	defer func() { _ = unix.SetNonblock(fd, false) }()

	buf := make([]byte, 4096)
	deadline := time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(deadline) {
		n, rerr := unix.Read(fd, buf)
		if n > 0 {
			continue // discard
		}
		if rerr == unix.EAGAIN || rerr == unix.EWOULDBLOCK {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
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
