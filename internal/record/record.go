// Package record spawns a shell under a PTY and records its output.
package record

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	"ttrack/internal/cast"
	"ttrack/internal/store"
)

func daemonSocket() string {
	if s := os.Getenv("TTRACKD_SOCK"); s != "" {
		return s
	}
	return "/run/ttrackd.sock"
}

// openSink picks where the recording goes: stream to the root daemon when
// reachable (central, root-only), else a user-local file (fail-open). An
// explicit -o always uses a local file at that path.
func openSink(out string) (io.WriteCloser, string, error) {
	if out == "" {
		if conn, derr := net.DialTimeout("unix", daemonSocket(), 1*time.Second); derr == nil {
			if _, werr := conn.Write([]byte("REC\n")); werr == nil {
				return conn, "ttrackd (central)", nil
			}
			_ = conn.Close()
		}
	}
	path := out
	if path == "" {
		p, err := store.NewPath()
		if err != nil {
			return nil, "", err
		}
		path = p
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

// Run records a session. args is the rec subcommand's argv (after "rec").
func Run(args []string) error {
	fs := flag.NewFlagSet("rec", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default: auto-named in store dir)")
	quietFlag := fs.Bool("q", false, "suppress the recording banner and saved-path message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	quiet := *quietFlag || os.Getenv("TTRACK_QUIET") != ""

	cmdArgs := fs.Args()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	if len(cmdArgs) == 0 {
		cmdArgs = []string{shell}
	}

	sink, dest, serr := openSink(*out)
	if serr != nil {
		return serr
	}
	defer sink.Close()

	cw, err := cast.NewWriter(sink, buildHeader(cmdArgs, shell))
	if err != nil {
		return err
	}

	if !quiet {
		fmt.Fprintf(os.Stderr,
			"ttrack: recording to %s — type 'exit' or Ctrl-D to stop\r\n", dest)
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }()

	winch := watchResize(ptmx)
	restore := makeRawRestore(int(os.Stdin.Fd()))
	defer restore()

	start := time.Now()
	var wg sync.WaitGroup

	// Local stdin -> PTY (user keystrokes). Not recorded.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()

	// PTY -> local stdout + recording.
	wg.Add(1)
	go pumpOutput(ptmx, cw, start, &wg)

	waitErr := cmd.Wait()
	signal.Stop(winch)
	_ = ptmx.Close() // unblock the reader goroutine
	wg.Wait()
	_ = cw.Close()
	restore()

	if !quiet {
		fmt.Fprintf(os.Stderr, "\r\nttrack: session saved to %s\n", dest)
	}

	// Surface the child's exit code without treating it as a ttrack error.
	if ee, ok := waitErr.(*exec.ExitError); ok {
		_ = ee
	}
	return nil
}

// buildHeader builds the cast header, using the current terminal size if available.
func buildHeader(cmdArgs []string, shell string) cast.Header {
	width, height := 80, 24
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		if w, h, err := term.GetSize(fd); err == nil {
			width, height = w, h
		}
	}
	return cast.Header{
		Width:     width,
		Height:    height,
		Timestamp: time.Now().Unix(),
		Command:   strings.Join(cmdArgs, " "),
		Env:       map[string]string{"SHELL": shell, "TERM": os.Getenv("TERM")},
	}
}

// watchResize forwards SIGWINCH to the PTY and syncs the initial size.
func watchResize(ptmx *os.File) chan os.Signal {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	winch <- syscall.SIGWINCH
	return winch
}

// makeRawRestore puts the terminal in raw mode and returns a restore func.
func makeRawRestore(fd int) func() {
	var oldState *term.State
	if term.IsTerminal(fd) {
		if st, err := term.MakeRaw(fd); err == nil {
			oldState = st
		}
	}
	return func() {
		if oldState != nil {
			_ = term.Restore(fd, oldState)
			oldState = nil
		}
	}
}

// pumpOutput copies PTY output to the local terminal and the recording.
// It flushes to the recording sink at most every flushInterval (and once at
// end) rather than on every chunk, so interactive typing stays snappy while
// live tail still sees output within ~flushInterval.
func pumpOutput(ptmx *os.File, cw *cast.Writer, start time.Time, wg *sync.WaitGroup) {
	defer wg.Done()
	const flushInterval = 100 * time.Millisecond
	buf := make([]byte, 32*1024)
	lastFlush := time.Now()
	for {
		n, rerr := ptmx.Read(buf)
		if n > 0 {
			_, _ = os.Stdout.Write(buf[:n])
			_ = cw.WriteOutput(time.Since(start).Seconds(), buf[:n])
			if time.Since(lastFlush) >= flushInterval {
				_ = cw.Flush()
				lastFlush = time.Now()
			}
		}
		if rerr != nil {
			_ = cw.Flush()
			return
		}
	}
}
