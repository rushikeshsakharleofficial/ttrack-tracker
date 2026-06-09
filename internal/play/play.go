// Package play replays a recorded cast file to the terminal.
package play

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"ttrack/internal/cast"
	"ttrack/internal/store"
)

const (
	hideCursor  = "\x1b[?25l"     // DECTCEM hide
	showCursor  = "\x1b[?25h"     // DECTCEM show
	altEnter    = "\x1b[?1049h"   // enter alternate screen (saves prior screen)
	altExit     = "\x1b[?1049l"   // leave alternate screen (restores prior screen)
	clearScreen = "\x1b[2J\x1b[H" // clear and home
	resetRegion = "\x1b[r"        // clear scroll region
	seekStep    = 5.0             // seconds per arrow-key seek
	maxSpeed    = 64.0
	minSpeed    = 1.0 / 64
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
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
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
	events = clampIdleGaps(events, maxIdle)

	interactive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	if !interactive {
		// stdin may still be a tty (e.g. output piped). Suppress echo and drain
		// any terminal query responses so they don't leak onto the shell prompt.
		restore := beginReplayInput()
		err = playLinear(events, speed)
		drainTerminalInput()
		if restore != nil {
			restore()
		}
		return err
	}
	return playInteractive(events, speed)
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

// clampIdleGaps returns a new event slice with inter-event gaps capped to
// maxIdle seconds. Timestamps are remapped so every downstream consumer
// (seek, chapters, clock) sees a consistent compressed timeline.
// Returns the original slice unchanged when maxIdle <= 0.
func clampIdleGaps(events []cast.Event, maxIdle float64) []cast.Event {
	if maxIdle <= 0 || len(events) == 0 {
		return events
	}
	out := make([]cast.Event, len(events))
	copy(out, events)
	var offset, last float64
	for i, ev := range out {
		gap := ev.Time - last
		last = ev.Time
		if gap > maxIdle {
			offset += gap - maxIdle
		}
		out[i].Time = ev.Time - offset
	}
	return out
}

// playLinear plays every event straight through with original timing, used
// when stdout is not a terminal (piping, redirection).
func playLinear(events []cast.Event, speed float64) error {
	if speed <= 0 {
		speed = 1.0
	}
	fmt.Fprintln(os.Stderr, "--- ttrack replay start ---")
	defer fmt.Fprintln(os.Stderr, "\r\n--- ttrack replay end ---")

	var last float64
	for _, ev := range events {
		gap := ev.Time - last
		last = ev.Time
		if gap > 0 {
			time.Sleep(time.Duration(gap / speed * float64(time.Second)))
		}
		if ev.Type == "o" {
			if _, err := io.WriteString(os.Stdout, ev.Data); err != nil {
				return err
			}
		}
	}
	return nil
}

// playInteractive replays inside a full-screen player frame: an alternate
// screen with the recording rendered in a scroll region above a persistent
// bottom transport bar (progress, time, speed). Controls: space pause/resume,
// arrows or h/l seek, up/down or +/- speed, g goto prompt, mouse-click the bar
// to seek, q quit. Playback holds on the final frame at the end until quit.
func playInteractive(events []cast.Event, speed float64) error {
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
		e := playLinear(events, speed)
		drainTerminalInput()
		if r != nil {
			r()
		}
		return e
	}

	_, h := termSize()
	setRegion := func(height int) { fmt.Fprintf(os.Stdout, "\x1b[1;%dr", height-1) }

	// Enter the player frame: alt screen, clear, hide cursor, reserve the bottom
	// row for the transport bar by confining output to a scroll region above it.
	_, _ = io.WriteString(os.Stdout, altEnter+clearScreen+hideCursor)
	setRegion(h)
	_, _ = io.WriteString(os.Stdout, mouseEnable)

	var once sync.Once
	restore := func() {
		once.Do(func() {
			_, _ = io.WriteString(os.Stdout, resetRegion+mouseDisable+altExit+showCursor)
			_ = term.Restore(fd, old)
		})
	}
	defer restore()

	// MakeRaw disables ISIG, so Ctrl-C arrives as a byte (handled in-band).
	// Keep a SIGTERM handler so `kill` still restores the terminal.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigc)
		close(sigc)
	}()

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer func() {
		signal.Stop(winch)
		close(winch)
	}()

	evc := make(chan event, 256)
	go readInput(evc)

	var lastT float64
	if n := len(events); n > 0 {
		lastT = events[n-1].Time
	}
	idx := 0                   // index of the next event to emit
	vt := 0.0                  // playback time of the last emitted event
	anchor := time.Now()       // wall time vt was last synced to
	saveDepth := 0             // recording's open DECSC (\x1b7) count; bar heal waits for 0
	var saveOpenedAt time.Time // when saveDepth last went positive (heal decay)
	paused := false
	inGoto := false
	gotoBuf := ""
	listMode := false
	barHidden := false
	var chapters []chapter
	haveChapters := false
	listSel := 0
	barCol, barW, barRow := 0, 0, h

	// Scrollback state.
	var scrollBuf lineBuf
	scrollMode := false
	scrollOffset := 0
	var renderBuf *strings.Builder // non-nil during renderTo → redirects emit to buffer

	// frozen reports whether playback is held (any modal/paused state).
	frozen := func() bool { return paused || inGoto || listMode || scrollMode }

	// displayTime is vt interpolated by wall time while playing, so the clock
	// advances smoothly (including across long idle gaps) without mutating vt.
	displayTime := func() float64 {
		if frozen() {
			return vt
		}
		d := vt + time.Since(anchor).Seconds()*speed
		if d > lastT {
			d = lastT
		}
		return d
	}
	drawBar := func() {
		if barHidden {
			return
		}
		barCol, barW, barRow = drawStatus(displayTime(), lastT, speed, paused, inGoto, gotoBuf)
	}
	// safeDrawBar heals the bar only when the recording isn't mid save/restore,
	// so our cursor save/restore can't corrupt the recording's own (which can
	// span events, e.g. apt progress bars). A stale open save (truncated cast,
	// mismatched \x1b7) decays after 500ms so the clock can't freeze forever.
	safeDrawBar := func() {
		if saveDepth <= 0 || time.Since(saveOpenedAt) >= 500*time.Millisecond {
			drawBar()
		}
	}
	emit := func(data string) {
		if renderBuf != nil {
			renderBuf.WriteString(data)
		} else {
			_, _ = io.WriteString(os.Stdout, data)
		}
		scrollBuf.feed(data)
		d, reset := saveDelta(data)
		if reset { // RIS in the stream clears the save slot
			saveDepth = 0
			return
		}
		prev := saveDepth
		saveDepth += d
		if saveDepth < 0 {
			saveDepth = 0
		}
		if prev == 0 && saveDepth > 0 {
			saveOpenedAt = time.Now()
		}
	}
	emitForward := func(target float64) {
		for idx < len(events) && events[idx].Time <= target {
			if events[idx].Type == "o" {
				emit(events[idx].Data)
			}
			idx++
		}
	}
	// renderTo clears the viewport and replays from the start up to target.
	// Used for backward seeks, resizes, and exiting overlays; never RIS (that
	// would drop the alt screen and scroll region). Resets the scrollback buffer
	// so it matches the replayed output.
	renderTo := func(target float64) {
		var b strings.Builder
		b.WriteString(clearScreen)
		renderBuf = &b
		saveDepth = 0
		idx = 0
		scrollBuf = lineBuf{}
		emitForward(target)
		renderBuf = nil
		_, _ = io.WriteString(os.Stdout, b.String())
	}
	syncClock := func() {
		if !frozen() {
			vt = displayTime()
		}
		anchor = time.Now()
	}
	seek := func(target float64) {
		if target < 0 {
			target = 0
		}
		if target > lastT {
			target = lastT
		}
		if target < vt {
			renderTo(target)
		} else {
			emitForward(target)
		}
		vt = target
		anchor = time.Now()
	}
	// drawScrollView renders the scrollback buffer in the viewport. Resets the
	// scroll region to full-screen, clears, prints buffered lines, then draws the
	// scroll indicator at the bottom row. drawBar is NOT called here; exitScroll
	// calls it after restoring the scroll region.
	drawScrollView := func() {
		w, hh := termSize()
		contentH := hh - 1 // reserve 1 row for scroll indicator
		if contentH < 1 {
			contentH = 1
		}
		total := len(scrollBuf.lines)

		end := total - scrollOffset
		if end > total {
			end = total
		}
		if end < 0 {
			end = 0
		}
		start := end - contentH
		if start < 0 {
			start = 0
		}

		var b strings.Builder
		b.WriteString(resetRegion + clearScreen + "\x1b[0m")
		for i := start; i < end; i++ {
			line := scrollBuf.lines[i]
			if visWidth(line) > w {
				line = truncLine(line, w)
			}
			// \x1b[0m before: start each line with default attributes
			// \x1b[0m\x1b[K after: reset SGR then erase trailing cells with
			// default background, preventing color bleed into unfilled columns
			b.WriteString("\x1b[0m" + line + "\x1b[0m\x1b[K\r\n")
		}
		_, _ = io.WriteString(os.Stdout, b.String())
		drawScrollBar(total, scrollOffset, hh)
	}

	enterScroll := func() {
		if scrollMode {
			return
		}
		syncClock()
		paused = true
		scrollMode = true
		scrollOffset = 0
		drawScrollView()
	}

	exitScroll := func() {
		scrollMode = false
		setRegion(h)
		renderTo(vt)
		drawBar()
	}

	scrollUp := func() {
		if !scrollMode {
			enterScroll()
			return
		}
		_, hh := termSize()
		contentH := hh - 1
		if contentH < 1 {
			contentH = 1
		}
		total := len(scrollBuf.lines)
		maxOff := total - contentH
		if maxOff < 0 {
			maxOff = 0
		}
		newOff := scrollOffset + scrollStep
		if newOff > maxOff {
			newOff = maxOff
		}
		scrollOffset = newOff
		drawScrollView()
	}

	scrollDown := func() {
		if !scrollMode {
			return
		}
		newOff := scrollOffset - scrollStep
		if newOff < 0 {
			newOff = 0
		}
		scrollOffset = newOff
		if scrollOffset == 0 {
			exitScroll()
			return
		}
		drawScrollView()
	}

	resize := func() {
		_, h = termSize()
		if scrollMode {
			drawScrollView()
			return
		}
		if barHidden {
			_, _ = io.WriteString(os.Stdout, resetRegion)
		} else {
			setRegion(h)
		}
		renderTo(displayTime())
	}
	// toggleBar shows/hides the transport bar. Hiding releases the reserved row
	// so the recording renders full-height; showing reclaims it.
	toggleBar := func() {
		syncClock()
		barHidden = !barHidden
		if barHidden {
			_, _ = io.WriteString(os.Stdout, resetRegion)
		} else {
			setRegion(h)
		}
		renderTo(vt)
		drawBar()
	}

	openGotoList := func() {
		if !haveChapters {
			chapters = buildChapters(events)
			haveChapters = true
		}
		syncClock()
		if len(chapters) == 0 {
			inGoto, gotoBuf = true, "" // no commands detected; fall back to time entry
			return
		}
		listMode = true
		listSel = chapterAt(chapters, vt)
		w, hh := termSize()
		drawChapterList(chapters, listSel, w, hh)
	}
	exitList := func(target float64, jump bool) {
		listMode = false
		if jump {
			if target < 0 {
				target = 0
			}
			if target > lastT {
				target = lastT
			}
			vt = target
		}
		anchor = time.Now()
		renderTo(vt) // always clear the menu and rebuild content to vt
		drawBar()
	}

	dispatchList := func(ev event) bool {
		redraw := func() {
			w, hh := termSize()
			drawChapterList(chapters, listSel, w, hh)
		}
		switch ev.kind {
		case evMouse:
			if ev.press {
				_, hh := termSize()
				rows := hh - 2
				if rows < 1 {
					rows = 1
				}
				top := 0
				if listSel >= rows {
					top = listSel - rows + 1
				}
				if i := top + (ev.my - 2); ev.my >= 2 && i >= 0 && i < len(chapters) {
					listSel = i
					exitList(chapters[listSel].t, true) // single click jumps
				}
			}
		case evArrow:
			switch ev.b {
			case 'A':
				if listSel > 0 {
					listSel--
				}
				redraw()
			case 'B':
				if listSel < len(chapters)-1 {
					listSel++
				}
				redraw()
			}
		case evByte:
			switch ev.b {
			case 'k':
				if listSel > 0 {
					listSel--
				}
				redraw()
			case 'j':
				if listSel < len(chapters)-1 {
					listSel++
				}
				redraw()
			case 0x0d, 0x0a: // Enter — jump to the selected command
				exitList(chapters[listSel].t, true)
			case 't': // switch to manual time entry
				listMode = false
				inGoto, gotoBuf = true, ""
				renderTo(vt)
				drawBar()
			case 'q', 0x03: // back to the player
				exitList(0, false)
			}
		}
		return true
	}

	handleByte := func(b byte) bool {
		if inGoto {
			switch {
			case b == 0x0d || b == 0x0a: // Enter
				if t, ok := parseClock(gotoBuf); ok {
					seek(t)
				}
				inGoto, gotoBuf = false, ""
				anchor = time.Now()
			case b == 0x7f || b == 0x08: // Backspace
				if len(gotoBuf) > 0 {
					gotoBuf = gotoBuf[:len(gotoBuf)-1]
				}
			case (b >= '0' && b <= '9') || b == ':':
				gotoBuf += string(b)
			default: // any other key cancels the goto prompt
				inGoto, gotoBuf = false, ""
				anchor = time.Now()
			}
			return true
		}
		switch b {
		case 'q', 0x03: // q or Ctrl-C
			return false
		case ' ':
			if paused {
				paused = false
				anchor = time.Now()
			} else {
				syncClock()
				paused = true
			}
		case 'h':
			syncClock()
			seek(vt - seekStep)
		case 'l':
			syncClock()
			seek(vt + seekStep)
		case '+', '=':
			syncClock()
			speed = faster(speed)
		case '-', '_':
			syncClock()
			speed = slower(speed)
		case '0':
			seek(0)
		case 'g':
			openGotoList()
		case 'b':
			toggleBar()
		}
		return true
	}

	dispatch := func(ev event) bool {
		if listMode {
			return dispatchList(ev)
		}
		// Scroll mode: scroll keys navigate; any other key exits scroll mode.
		if scrollMode {
			switch ev.kind {
			case evScroll:
				if ev.up {
					scrollUp()
				} else {
					scrollDown()
				}
			default:
				exitScroll()
			}
			return true
		}
		switch ev.kind {
		case evScroll:
			if ev.up {
				scrollUp()
			} else {
				scrollDown()
			}
		case evMouse:
			if !inGoto && !barHidden && ev.press && ev.my == barRow && barW > 1 &&
				ev.mx >= barCol && ev.mx <= barCol+barW-1 {
				syncClock()
				frac := float64(ev.mx-barCol) / float64(barW-1)
				seek(frac * lastT)
			}
		case evArrow:
			if inGoto {
				return true
			}
			switch ev.b {
			case 'C':
				syncClock()
				seek(vt + seekStep)
			case 'D':
				syncClock()
				seek(vt - seekStep)
			case 'A':
				syncClock()
				speed = faster(speed)
			case 'B':
				syncClock()
				speed = slower(speed)
			}
		case evByte:
			return handleByte(ev.b)
		}
		return true
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	drawBar()
	for {
		// At the end, hold on the final frame (auto-pause) until the user quits.
		if !frozen() && idx >= len(events) {
			paused = true
			vt = lastT
			drawBar()
		}
		if frozen() {
			select {
			case ev, ok := <-evc:
				if !ok || !dispatch(ev) {
					return nil
				}
			case <-sigc:
				return errors.New("terminated")
			case <-winch:
				resize()
				if listMode {
					w, hh := termSize()
					drawChapterList(chapters, listSel, w, hh)
				}
			}
			if !listMode && !scrollMode {
				drawBar()
			}
			continue
		}
		next := events[idx].Time
		fireAt := anchor.Add(time.Duration((next - vt) / speed * float64(time.Second)))
		wait := time.Until(fireAt)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
			vt = next
			anchor = time.Now()
			if events[idx].Type == "o" {
				emit(events[idx].Data)
			}
			idx++
		case <-ticker.C:
			timer.Stop()
			safeDrawBar()
		case ev, ok := <-evc:
			timer.Stop()
			if !ok || !dispatch(ev) {
				return nil
			}
			if !listMode && !scrollMode {
				drawBar()
			}
		case <-sigc:
			timer.Stop()
			return errors.New("terminated")
		case <-winch:
			timer.Stop()
			resize()
			drawBar()
		}
	}
}

// saveDelta returns the net change in open cursor-save (DECSC \x1b7 / DECRC
// \x1b8) depth contained in data. Recordings sometimes split a save/restore
// pair across events; tracking the depth lets the bar avoid drawing between
// them (its own save/restore would otherwise clobber the recording's).
func saveDelta(s string) (delta int, reset bool) {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == 0x1b {
			switch s[i+1] {
			case '7': // DECSC
				delta++
			case '8': // DECRC
				delta--
			case 'c': // RIS clears the save slot
				delta, reset = 0, true
			}
		}
	}
	return delta, reset
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
