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
// recordings. On an interactive terminal it shows a video-player-style status
// bar with playback controls; otherwise it plays straight through.
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
	events, err := readCastEvents(r)
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

func readCastEvents(r *bufio.Reader) ([]cast.Event, error) {
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

// playInteractive replays with a status bar and keyboard/mouse controls:
// space pauses (showing the bar), arrows/h/l seek, up/down or +/- change speed,
// g opens a goto prompt, mouse-click on the bar seeks, q quits. The bar is only
// shown while paused (and as a brief toast on actions during playback) so it
// never fights a full-screen recording's own redraws; resuming re-renders the
// screen from the start up to the current time to wipe the overlay.
func playInteractive(events []cast.Event, speed, maxIdle float64) error {
	if speed <= 0 {
		speed = 1.0
	}
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		// Can't drive controls without raw mode. Fall back to a straight replay,
		// keeping the echo-off + drain protection so terminal query responses
		// don't leak onto the shell prompt.
		r := beginReplayInput()
		e := playLinear(events, speed, maxIdle)
		drainTerminalInput()
		if r != nil {
			r()
		}
		return e
	}
	var once sync.Once
	restore := func() {
		once.Do(func() {
			eraseStatusLine()
			_, _ = io.WriteString(os.Stdout, mouseDisable)
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

	_, _ = io.WriteString(os.Stdout, mouseEnable)

	evc := make(chan event, 256)
	go readInput(evc)

	if maxIdle > 0 {
		fmt.Fprintln(os.Stderr, "ttrack: --idle ignored in interactive replay (use seek)")
	}
	fmt.Fprintln(os.Stderr, "\r--- ttrack replay  [space=pause  ←/→=seek  ↑/↓=speed  g=goto  click=seek  q=quit] ---")
	defer fmt.Fprintln(os.Stderr, "\r\n--- ttrack replay end ---")

	var lastT float64
	if n := len(events); n > 0 {
		lastT = events[n-1].Time
	}
	idx := 0  // index of the next event to emit
	vt := 0.0 // virtual playback time (events with Time<=vt already emitted)
	paused := false
	inGoto := false
	gotoBuf := ""
	toastShown := false
	barCol, barW, barRow := 0, 0, 0

	emitForward := func(target float64) {
		for idx < len(events) && events[idx].Time <= target {
			if events[idx].Type == "o" {
				_, _ = io.WriteString(os.Stdout, events[idx].Data)
			}
			idx++
		}
	}
	// redrawScreen resets the terminal and replays all output up to target, so
	// the screen is correct for any seek and any overlay is wiped.
	redrawScreen := func(target float64) {
		_, _ = io.WriteString(os.Stdout, resetTerm)
		idx = 0
		emitForward(target)
	}
	seek := func(target float64) {
		if target < 0 {
			target = 0
		}
		if target > lastT {
			target = lastT
		}
		if target < vt {
			redrawScreen(target)
		} else {
			emitForward(target)
		}
		vt = target
	}
	showBar := func() {
		barCol, barW, barRow = drawStatus(vt, lastT, speed, paused, inGoto, gotoBuf)
	}
	// feedback reflects a control action: redraw the bar while paused, else show
	// a brief toast that the next emitted event clears.
	feedback := func() {
		if paused {
			showBar()
		} else {
			drawStatus(vt, lastT, speed, false, false, "")
			toastShown = true
		}
	}
	enterPause := func() {
		paused = true
		_, _ = io.WriteString(os.Stdout, mouseEnable) // re-assert; replay may have toggled it
		showBar()
	}
	leavePause := func() {
		paused = false
		inGoto = false
		gotoBuf = ""
		redrawScreen(vt) // wipe the bar, restore content to the current time
	}
	commitGoto := func() {
		if t, ok := parseClock(gotoBuf); ok {
			seek(t)
		}
		inGoto = false
		gotoBuf = ""
		showBar()
	}

	handleByte := func(b byte) bool {
		if inGoto {
			switch {
			case b == 0x0d || b == 0x0a: // Enter
				commitGoto()
			case b == 0x7f || b == 0x08: // Backspace
				if len(gotoBuf) > 0 {
					gotoBuf = gotoBuf[:len(gotoBuf)-1]
				}
				showBar()
			case (b >= '0' && b <= '9') || b == ':':
				gotoBuf += string(b)
				showBar()
			default: // any other key cancels the goto prompt
				inGoto = false
				gotoBuf = ""
				showBar()
			}
			return true
		}
		switch b {
		case 'q', 0x03: // q or Ctrl-C
			return false
		case ' ':
			if paused {
				leavePause()
			} else {
				enterPause()
			}
		case 'h':
			seek(vt - seekStep)
			feedback()
		case 'l':
			seek(vt + seekStep)
			feedback()
		case '+', '=':
			speed = faster(speed)
			feedback()
		case '-', '_':
			speed = slower(speed)
			feedback()
		case '0':
			seek(0)
			feedback()
		case 'g':
			if paused {
				inGoto = true
				gotoBuf = ""
				showBar()
			}
		}
		return true
	}

	dispatch := func(ev event) bool {
		switch ev.kind {
		case evMouse:
			if paused && !inGoto && ev.press && ev.my == barRow && barW > 1 &&
				ev.mx >= barCol && ev.mx <= barCol+barW-1 {
				frac := float64(ev.mx-barCol) / float64(barW-1)
				seek(frac * lastT)
				showBar()
			}
		case evArrow:
			if inGoto {
				return true
			}
			switch ev.b {
			case 'C':
				seek(vt + seekStep)
			case 'D':
				seek(vt - seekStep)
			case 'A':
				speed = faster(speed)
			case 'B':
				speed = slower(speed)
			}
			feedback()
		case evByte:
			return handleByte(ev.b)
		}
		return true
	}

	for idx < len(events) {
		if paused {
			ev, ok := <-evc
			if !ok || !dispatch(ev) {
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
			if toastShown {
				eraseStatusLine()
				toastShown = false
			}
			vt = next
			if events[idx].Type == "o" {
				_, _ = io.WriteString(os.Stdout, events[idx].Data)
			}
			idx++
		case ev, ok := <-evc:
			timer.Stop()
			vt += time.Since(start).Seconds() * speed
			if vt > next {
				vt = next
			}
			if !ok || !dispatch(ev) {
				return nil
			}
		}
	}
	return nil
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
