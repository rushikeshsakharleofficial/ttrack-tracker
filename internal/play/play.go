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
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"ttrack/internal/cast"
	"ttrack/internal/store"
)

const (
	showCursor = "\x1b[?25h" // DECTCEM show — replayed TUIs often hide and never restore
	resetTerm  = "\x1bc"     // RIS — full reset before re-rendering on a backward seek
	seekStep   = 5.0         // seconds per arrow-key seek
	maxSpeed   = 64.0
	minSpeed   = 1.0 / 64
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

// PlayFile replays the cast at path, transparently decrypting encrypted
// recordings. On an interactive terminal it offers playback controls
// (pause, seek, speed); otherwise it plays straight through.
func PlayFile(path string, speed, maxIdle float64) error {
	if speed <= 0 {
		speed = 1.0
	}
	// Snapshot at open time: replaying an in-progress recording is bounded to
	// the data present when playback began (seeking needs a fixed length).
	rc, err := store.OpenCastSnapshot(path)
	if err != nil {
		return err
	}
	defer rc.Close()

	r := bufio.NewReader(rc)
	if _, err := cast.ReadHeader(r); err != nil {
		return err
	}
	events, err := readEvents(r)
	if err != nil {
		return err
	}

	interactive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	if !interactive {
		// stdin may still be a tty (e.g. output piped). Suppress echo and drain
		// any terminal query responses so they don't leak onto the shell prompt.
		restore := beginReplayInput()
		err = playLinear(events, speed, maxIdle)
		drainTerminalInput()
		if restore != nil {
			restore()
		}
		return err
	}
	return playInteractive(events, speed, maxIdle)
}

func readEvents(r *bufio.Reader) ([]cast.Event, error) {
	var events []cast.Event
	for {
		ev, err := cast.ReadEvent(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

// playLinear plays every event straight through with original timing, used
// when stdout is not a terminal (piping, redirection).
func playLinear(events []cast.Event, speed, maxIdle float64) error {
	if speed <= 0 {
		speed = 1.0
	}
	fmt.Fprintln(os.Stderr, "--- ttrack replay start ---")
	defer fmt.Fprintln(os.Stderr, "\r\n--- ttrack replay end ---")

	var last float64
	for _, ev := range events {
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

// playInteractive replays with keyboard controls: space pauses, left/right
// seek 5s, up/down change speed, q quits. A backward seek re-renders the
// screen from the start of the recording up to the target time.
func playInteractive(events []cast.Event, speed, maxIdle float64) error {
	if speed <= 0 {
		speed = 1.0
	}
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return playLinear(events, speed, maxIdle)
	}
	var once sync.Once
	restore := func() {
		once.Do(func() {
			_ = term.Restore(fd, old)
			_, _ = io.WriteString(os.Stdout, showCursor)
		})
	}
	defer restore()

	// MakeRaw disables ISIG, so Ctrl-C arrives as a byte (handled in-band).
	// Keep a SIGTERM handler so `kill` still restores the terminal.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	defer signal.Stop(sigc)
	go func() {
		if _, ok := <-sigc; ok {
			restore()
			os.Exit(143)
		}
	}()

	cmds := make(chan ctrl, 256)
	go readKeys(cmds)

	if maxIdle > 0 {
		fmt.Fprintln(os.Stderr, "ttrack: --idle ignored in interactive replay (use seek)")
	}
	fmt.Fprintln(os.Stderr, "\r--- ttrack replay  [space=pause  ←/→=seek 5s  ↑/↓=speed  0=restart  q=quit] ---")
	defer fmt.Fprintln(os.Stderr, "\r\n--- ttrack replay end ---")

	var lastT float64
	if n := len(events); n > 0 {
		lastT = events[n-1].Time
	}
	idx := 0    // index of the next event to emit
	vt := 0.0   // virtual playback time (events with Time<=vt already emitted)
	paused := false

	emitForward := func(target float64) {
		for idx < len(events) && events[idx].Time <= target {
			if events[idx].Type == "o" {
				_, _ = io.WriteString(os.Stdout, events[idx].Data)
			}
			idx++
		}
	}
	seek := func(target float64) {
		if target < 0 {
			target = 0
		}
		if target > lastT {
			target = lastT
		}
		if target < vt {
			// Backward: terminal state is cumulative, so reset and replay all
			// output from the start up to the target instantly.
			_, _ = io.WriteString(os.Stdout, resetTerm)
			idx = 0
		}
		emitForward(target)
		vt = target
	}
	// handle returns false when playback should stop.
	handle := func(c ctrl) bool {
		switch c {
		case ctrlQuit:
			return false
		case ctrlPause:
			paused = !paused
		case ctrlFwd:
			seek(vt + seekStep)
		case ctrlBack:
			seek(vt - seekStep)
		case ctrlFaster:
			if speed < maxSpeed {
				speed *= 2
			}
		case ctrlSlower:
			if speed > minSpeed {
				speed /= 2
			}
		case ctrlRestart:
			seek(0)
		}
		return true
	}

	for idx < len(events) {
		if paused {
			c, ok := <-cmds
			if !ok || !handle(c) {
				return nil
			}
			continue
		}
		next := events[idx].Time
		wait := time.Duration((next - vt) / speed * float64(time.Second))
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		start := time.Now()
		select {
		case <-timer.C:
			vt = next
			if events[idx].Type == "o" {
				_, _ = io.WriteString(os.Stdout, events[idx].Data)
			}
			idx++
		case c, ok := <-cmds:
			timer.Stop()
			vt += time.Since(start).Seconds() * speed
			if vt > next {
				vt = next
			}
			if !ok || !handle(c) {
				return nil
			}
		}
	}
	return nil
}

// ctrl is a high-level playback control derived from keyboard input.
type ctrl int

const (
	ctrlPause ctrl = iota
	ctrlQuit
	ctrlFwd
	ctrlBack
	ctrlFaster
	ctrlSlower
	ctrlRestart
)

// readKeys reads stdin and emits playback controls. It runs a small escape
// parser so that the terminal's own replies to query sequences echoed during
// replay (Device Attributes, cursor reports, OSC color answers — all
// ESC-prefixed) are swallowed instead of being mistaken for user keystrokes.
func readKeys(out chan<- ctrl) {
	var p keyParser
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		for i := 0; i < n; i++ {
			p.feed(buf[i], out)
		}
		if err != nil {
			close(out)
			return
		}
	}
}

type parseState int

const (
	stGround parseState = iota
	stEsc
	stCSI
	stSS3
	stOSC
	stOSCEsc
)

// keyParser is a minimal terminal-input state machine. It recognises only the
// keys ttrack cares about and discards everything else, including complete
// escape sequences sent by the terminal in response to replayed queries.
type keyParser struct {
	state    parseState
	csiParam bool // CSI carried parameter/intermediate bytes (so it isn't a bare arrow)
}

func (p *keyParser) feed(b byte, out chan<- ctrl) {
	switch p.state {
	case stGround:
		switch b {
		case 0x1b:
			p.state = stEsc
		case ' ':
			out <- ctrlPause
		case 'q', 0x03: // q or Ctrl-C
			out <- ctrlQuit
		case 'h':
			out <- ctrlBack
		case 'l':
			out <- ctrlFwd
		case '+', '=':
			out <- ctrlFaster
		case '-', '_':
			out <- ctrlSlower
		case '0':
			out <- ctrlRestart
		}
	case stEsc:
		switch b {
		case '[':
			p.state = stCSI
			p.csiParam = false
		case 'O': // SS3 — application-keypad arrows (ESC O A..D)
			p.state = stSS3
		case ']':
			p.state = stOSC
		default:
			p.state = stGround
		}
	case stCSI:
		if b >= 0x40 && b <= 0x7e { // final byte
			if !p.csiParam {
				emitArrow(b, out)
			}
			p.state = stGround
		} else {
			p.csiParam = true // params/intermediates → not a plain arrow (e.g. a DA/DSR reply)
		}
	case stSS3:
		emitArrow(b, out)
		p.state = stGround
	case stOSC:
		switch b {
		case 0x07: // BEL terminates OSC
			p.state = stGround
		case 0x1b: // possible ST (ESC \)
			p.state = stOSCEsc
		}
	case stOSCEsc:
		if b == '\\' {
			p.state = stGround
		} else {
			// Back-to-back sequence: the ESC began a new one. Re-dispatch.
			p.state = stEsc
			p.feed(b, out)
		}
	}
}

func emitArrow(final byte, out chan<- ctrl) {
	switch final {
	case 'A':
		out <- ctrlFaster // up
	case 'B':
		out <- ctrlSlower // down
	case 'C':
		out <- ctrlFwd // right
	case 'D':
		out <- ctrlBack // left
	}
}

// beginReplayInput puts stdin in a no-echo, non-canonical mode (keeping signal
// keys like Ctrl-C working) so terminal query responses aren't echoed to the
// screen during a non-interactive replay. Returns a restore func, or nil if
// stdin isn't a tty.
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
