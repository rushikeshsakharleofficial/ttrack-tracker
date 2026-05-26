// Package play replays a recorded cast file to the terminal.
package play

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"ttrack/internal/cast"
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

	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	if _, err := cast.ReadHeader(r); err != nil {
		return err
	}

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
		if *maxIdle > 0 && gap > *maxIdle {
			gap = *maxIdle
		}
		if gap > 0 {
			time.Sleep(time.Duration(gap / *speed * float64(time.Second)))
		}
		if ev.Type == "o" {
			_, _ = io.WriteString(os.Stdout, ev.Data)
		}
	}
	return nil
}
