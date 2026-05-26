// Package audit implements the root-only commands that read the central
// store: listing users and their sessions, replaying a session by id, live
// tailing an in-progress session, and a tree view.
package audit

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

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
