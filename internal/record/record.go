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

	// Choose the recording sink:
	//   - explicit -o: always a local file at that path.
	//   - otherwise: stream to the root daemon if reachable (central, root-only);
	//     fall back to a user-local file if the daemon is down (fail-open).
	var sink io.WriteCloser
	var dest string // human description for the banner
	if *out == "" {
		if conn, derr := net.DialTimeout("unix", daemonSocket(), 1*time.Second); derr == nil {
			if _, werr := conn.Write([]byte("REC\n")); werr == nil {
				sink = conn
				dest = "ttrackd (central)"
			} else {
				_ = conn.Close()
			}
		}
	}
	if sink == nil {
		path := *out
		if path == "" {
			p, err := store.NewPath()
			if err != nil {
				return err
			}
			path = p
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		sink = f
		dest = path
	}
	defer sink.Close()

	stdinFd := int(os.Stdin.Fd())
	width, height := 80, 24
	if term.IsTerminal(stdinFd) {
		if w, h, err := term.GetSize(stdinFd); err == nil {
			width, height = w, h
		}
	}

	cw, err := cast.NewWriter(sink, cast.Header{
		Width:     width,
		Height:    height,
		Timestamp: time.Now().Unix(),
		Command:   strings.Join(cmdArgs, " "),
		Env: map[string]string{
			"SHELL": shell,
			"TERM":  os.Getenv("TERM"),
		},
	})
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

	// Propagate window-size changes to the PTY.
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	winch <- syscall.SIGWINCH // sync initial size

	// Put the local terminal in raw mode so keystrokes pass through.
	var oldState *term.State
	if term.IsTerminal(stdinFd) {
		if st, err := term.MakeRaw(stdinFd); err == nil {
			oldState = st
		}
	}
	restore := func() {
		if oldState != nil {
			_ = term.Restore(stdinFd, oldState)
			oldState = nil
		}
	}
	defer restore()

	start := time.Now()
	var wg sync.WaitGroup

	// Local stdin -> PTY (user keystrokes). Not recorded.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()

	// PTY -> local stdout + recording.
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				_, _ = os.Stdout.Write(buf[:n])
				_ = cw.WriteOutput(time.Since(start).Seconds(), buf[:n])
			}
			if rerr != nil {
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	signal.Stop(winch)
	_ = ptmx.Close() // unblock the reader goroutine
	wg.Wait()
	_ = cw.Flush()
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
