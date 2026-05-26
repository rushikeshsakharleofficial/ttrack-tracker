// Package audit implements the root-only commands that read the central
// store: listing users and their sessions, replaying a session by id, live
// tailing an in-progress session, and a tree view.
package audit

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ttrack/internal/cast"
	"ttrack/internal/play"
	"ttrack/internal/store"
)

func daemonSocket() string {
	if s := os.Getenv("TTRACKD_SOCK"); s != "" {
		return s
	}
	return "/run/ttrackd.sock"
}

func notRoot(err error) error {
	if os.IsPermission(err) || os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s (run as root): %v", store.CentralDir(), err)
	}
	return err
}

// LsUser handles `ttrack ls-user [username]`.
func LsUser(args []string) error {
	if len(args) == 0 {
		users, err := store.Users()
		if err != nil {
			return notRoot(err)
		}
		if len(users) == 0 {
			fmt.Printf("no recorded users in %s\n", store.CentralDir())
			return nil
		}
		fmt.Printf("%-20s  %s\n", "USER", "SESSIONS")
		for _, u := range users {
			s, _ := store.UserSessions(u)
			fmt.Printf("%-20s  %d\n", u, len(s))
		}
		return nil
	}

	user := args[0]
	sessions, err := store.UserSessions(user)
	if err != nil {
		return notRoot(err)
	}
	if len(sessions) == 0 {
		fmt.Printf("no sessions for user %q\n", user)
		return nil
	}
	fmt.Printf("%-7s  %-26s  %-19s  %s\n", "STATUS", "SESSION", "STARTED", "COMMAND")
	for _, name := range sessions {
		h, _ := store.Header(centralPath(user, name))
		status := "SAVED"
		if store.IsActive(name) {
			status = "ACTIVE"
		}
		fmt.Printf("%-7s  %-26s  %-19s  %s\n",
			status, name, store.Started(h), h.Command)
	}
	return nil
}

// PlayUser handles `ttrack play-user [--speed N] [--idle N] <sessionid>`.
func PlayUser(args []string) error {
	fs := flag.NewFlagSet("play-user", flag.ContinueOnError)
	speed := fs.Float64("speed", 1.0, "playback speed multiplier")
	idle := fs.Float64("idle", 2.0, "cap idle gaps to N seconds (0 = no cap)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ttrack play-user [--speed N] [--idle N] <sessionid>")
	}
	path, user, err := store.FindCentral(fs.Arg(0))
	if err != nil {
		return notRoot(err)
	}
	fmt.Fprintf(os.Stderr, "--- session %s (user %s) ---\n", fs.Arg(0), user)
	return play.PlayFile(path, *speed, *idle)
}

// Export handles `ttrack export [-o file] <sessionid>` — decrypts a central
// recording to a plaintext asciinema v2 cast (for offline use / `asciinema play`).
func Export(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ttrack export [-o file] <sessionid>")
	}
	path, _, err := store.FindCentral(fs.Arg(0))
	if err != nil {
		return notRoot(err)
	}
	rc, err := store.OpenCast(path)
	if err != nil {
		return err
	}
	defer rc.Close()

	var w io.Writer = os.Stdout
	if *out != "" {
		f, ferr := os.Create(*out)
		if ferr != nil {
			return ferr
		}
		defer f.Close()
		w = f
	}
	if _, err := io.Copy(w, rc); err != nil {
		return err
	}
	if *out != "" {
		fmt.Fprintf(os.Stderr, "exported plaintext cast to %s\n", *out)
	}
	return nil
}

// Tree handles `ttrack tree` — users -> sessions.
func Tree(args []string) error {
	users, err := store.Users()
	if err != nil {
		return notRoot(err)
	}
	fmt.Println(store.CentralDir())
	for ui, u := range users {
		ubranch, uindent := "├─", "│  "
		if ui == len(users)-1 {
			ubranch, uindent = "└─", "   "
		}
		fmt.Printf("%s %s\n", ubranch, u)
		sessions, _ := store.UserSessions(u)
		for si, name := range sessions {
			sbranch := "├─"
			if si == len(sessions)-1 {
				sbranch = "└─"
			}
			h, _ := store.Header(centralPath(u, name))
			status := "SAVED"
			if store.IsActive(name) {
				status = "ACTIVE"
			}
			fmt.Printf("%s%s %s  [%s]  %s  %s\n",
				uindent, sbranch, name, status, store.Started(h), h.Command)
		}
	}
	return nil
}

// Tail handles `ttrack tail <sessionid>` — live stream from the daemon (root).
func Tail(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ttrack tail <sessionid>")
	}
	conn, err := net.Dial("unix", daemonSocket())
	if err != nil {
		return fmt.Errorf("ttrackd not reachable: %w", err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "TAIL %s\n", args[0]); err != nil {
		return err
	}
	_, err = io.Copy(os.Stdout, conn)
	return err
}

func centralPath(user, name string) string {
	return filepath.Join(store.CentralDir(), user, name)
}

// Search handles `ttrack search [--from T] [--to T] [--user U] [-i] <pattern>`.
// It scans the central store for recordings whose output (or recorded command)
// contains the pattern, optionally limited to sessions started in a time range.
func Search(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	from := fs.String("from", "", "only sessions started at/after this time")
	to := fs.String("to", "", "only sessions started at/before this time")
	userFilter := fs.String("user", "", "limit to one user")
	ignore := fs.Bool("i", false, "case-insensitive match")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ttrack search [--from T] [--to T] [--user U] [-i] <pattern>")
	}
	pattern := fs.Arg(0)
	needle := pattern
	if *ignore {
		needle = strings.ToLower(needle)
	}

	var fromT, toT time.Time
	var err error
	if *from != "" {
		if fromT, err = parseTime(*from); err != nil {
			return fmt.Errorf("bad --from: %w", err)
		}
	}
	if *to != "" {
		if toT, err = parseTime(*to); err != nil {
			return fmt.Errorf("bad --to: %w", err)
		}
	}

	users, err := store.Users()
	if err != nil {
		return notRoot(err)
	}

	matched := 0
	for _, u := range users {
		if *userFilter != "" && u != *userFilter {
			continue
		}
		names, _ := store.UserSessions(u)
		for _, name := range names {
			path := centralPath(u, name)
			h, herr := store.Header(path)
			if herr != nil {
				continue
			}
			if h.Timestamp > 0 {
				st := time.Unix(h.Timestamp, 0)
				if !fromT.IsZero() && st.Before(fromT) {
					continue
				}
				if !toT.IsZero() && st.After(toT) {
					continue
				}
			}
			cmdMatch, snips := scanCast(path, needle, *ignore, h)
			if !cmdMatch && len(snips) == 0 {
				continue
			}
			matched++
			// Lead with WHO ran it and WHEN, then the command and matches.
			fmt.Printf("user=%s  when=%s  session=%s\n",
				u, store.Started(h), strings.TrimSuffix(name, ".cast"))
			fmt.Printf("    cmd: %s\n", clean(h.Command))
			for _, s := range snips {
				fmt.Printf("    > %s\n", s)
			}
		}
	}
	if matched == 0 {
		fmt.Printf("no matches for %q\n", pattern)
	}
	return nil
}

// scanCast reports whether the recorded command matched, plus up to a few
// cleaned output snippet lines containing needle.
func scanCast(path, needle string, ignore bool, h cast.Header) (cmdMatch bool, snips []string) {
	const maxSnip = 5

	hay := h.Command
	if ignore {
		hay = strings.ToLower(hay)
	}
	if needle != "" && strings.Contains(hay, needle) {
		cmdMatch = true
	}

	rc, err := store.OpenCast(path)
	if err != nil {
		return cmdMatch, snips
	}
	defer rc.Close()
	r := bufio.NewReader(rc)
	if _, err := cast.ReadHeader(r); err != nil {
		return cmdMatch, snips
	}
	for len(snips) < maxSnip {
		ev, err := cast.ReadEvent(r)
		if err != nil {
			break
		}
		if ev.Type != "o" {
			continue
		}
		for _, line := range strings.Split(ev.Data, "\n") {
			h2 := line
			if ignore {
				h2 = strings.ToLower(h2)
			}
			if strings.Contains(h2, needle) {
				if c := clean(line); c != "" {
					snips = append(snips, c)
				}
				if len(snips) >= maxSnip {
					break
				}
			}
		}
	}
	return cmdMatch, snips
}

// parseTime accepts YYYY-MM-DD[ HH:MM[:SS]] or RFC3339, in local time.
func parseTime(s string) (time.Time, error) {
	for _, l := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q (use YYYY-MM-DD[ HH:MM[:SS]] or RFC3339)", s)
}

// clean strips CR and ANSI/control sequences for readable snippet display.
func clean(s string) string {
	var b strings.Builder
	ansi := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			ansi = true
			continue
		}
		if ansi {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				ansi = false
			}
			continue
		}
		if c == '\t' || (c >= 0x20 && c < 0x7f) {
			b.WriteByte(c)
		}
	}
	return strings.TrimSpace(b.String())
}
