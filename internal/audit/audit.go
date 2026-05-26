// Package audit implements the root-only commands that read the central
// store: listing users and their sessions, replaying a session by id, live
// tailing an in-progress session, and a tree view.
package audit

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

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
	idle := fs.Float64("idle", 0, "cap idle gaps to N seconds (default 0 = exact original timing)")
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
		ubranch, uindent := treeBranch(ui == len(users)-1)
		fmt.Printf("%s %s\n", ubranch, u)
		printUserSessions(u, uindent)
	}
	return nil
}

func treeBranch(last bool) (mark, indent string) {
	if last {
		return "└─", "   "
	}
	return "├─", "│  "
}

func printUserSessions(user, indent string) {
	sessions, _ := store.UserSessions(user)
	for si, name := range sessions {
		sbranch, _ := treeBranch(si == len(sessions)-1)
		h, _ := store.Header(centralPath(user, name))
		status := "SAVED"
		if store.IsActive(name) {
			status = "ACTIVE"
		}
		fmt.Printf("%s%s %s  [%s]  %s  %s\n",
			indent, sbranch, name, status, store.Started(h), h.Command)
	}
}

// Prune handles `ttrack prune` — interactively delete recordings from the
// central store by user and time range. Root only.
func Prune(args []string) error {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip the final confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}

	users, err := store.Users()
	if err != nil {
		return notRoot(err)
	}
	if len(users) == 0 {
		fmt.Printf("no recordings in %s\n", store.CentralDir())
		return nil
	}

	in := bufio.NewReader(os.Stdin)

	// 0. Show what's in the store, then require the prune password.
	printStorageOverview(users)
	if err := prunePasswordGate(in); err != nil {
		return err
	}

	// 1. Which user(s)?
	scope, err := resolveScope(ask(in, "Prune which user? [all / <username>]", "all"), users)
	if err != nil {
		return err
	}

	// 2. How much / time range?
	fmt.Println("What to delete:")
	fmt.Println("  all              every session for the selected user(s)")
	fmt.Println("  days N           sessions older than N days")
	fmt.Println("  range FROM TO    sessions started in [FROM, TO]  (YYYY-MM-DD[ HH:MM])")
	matchFn, err := pruneFilter(ask(in, "Selection?", "all"))
	if err != nil {
		return err
	}

	// 3. Collect matches.
	hits, total := collectPruneTargets(scope, matchFn)
	if len(hits) == 0 {
		fmt.Println("nothing matched — nothing to prune")
		return nil
	}

	// 4. Preview + confirm + delete.
	previewTargets(hits, total)
	if !*yes && !confirm(in, len(hits)) {
		fmt.Println("aborted — nothing deleted")
		return nil
	}
	deleted := deleteTargets(hits)
	fmt.Printf("pruned %d session(s), freed %s\n", deleted, humanSize(total))
	return nil
}

func previewTargets(hits []pruneTarget, total int64) {
	fmt.Printf("\nWill delete %d session(s), %s total:\n", len(hits), humanSize(total))
	for i, t := range hits {
		if i >= 20 {
			fmt.Printf("  ... and %d more\n", len(hits)-20)
			break
		}
		fmt.Printf("  %s/%s\n", t.user, t.name)
	}
}

func confirm(in *bufio.Reader, n int) bool {
	c := ask(in, fmt.Sprintf("Delete these %d session(s)? [yes/NO]", n), "no")
	return c == "yes" || c == "y"
}

func deleteTargets(hits []pruneTarget) int {
	deleted := 0
	for _, t := range hits {
		if err := os.Remove(t.path); err == nil {
			deleted++
		} else {
			fmt.Fprintf(os.Stderr, "  failed: %s: %v\n", t.path, err)
		}
	}
	return deleted
}

type pruneTarget struct {
	user, name, path string
	size             int64
}

func resolveScope(who string, users []string) ([]string, error) {
	if who == "all" {
		return users, nil
	}
	for _, u := range users {
		if u == who {
			return []string{who}, nil
		}
	}
	return nil, fmt.Errorf("no such user %q", who)
}

func pruneFilter(mode string) (func(ts int64) bool, error) {
	switch {
	case mode == "all":
		return func(int64) bool { return true }, nil
	case strings.HasPrefix(mode, "days"):
		n, e := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(mode, "days")))
		if e != nil || n < 0 {
			return nil, fmt.Errorf("bad 'days' value: %q", mode)
		}
		cutoff := time.Now().AddDate(0, 0, -n)
		return func(ts int64) bool { return ts > 0 && time.Unix(ts, 0).Before(cutoff) }, nil
	case strings.HasPrefix(mode, "range"):
		parts := strings.Fields(mode)
		if len(parts) < 3 {
			return nil, fmt.Errorf("usage: range FROM TO")
		}
		fromT, e1 := parseTime(parts[1])
		toT, e2 := parseTime(parts[2])
		if e1 != nil || e2 != nil {
			return nil, fmt.Errorf("bad range times")
		}
		return func(ts int64) bool {
			if ts == 0 {
				return false
			}
			t := time.Unix(ts, 0)
			return !t.Before(fromT) && !t.After(toT)
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized selection %q", mode)
	}
}

func collectPruneTargets(scope []string, match func(int64) bool) ([]pruneTarget, int64) {
	var hits []pruneTarget
	var total int64
	for _, u := range scope {
		names, _ := store.UserSessions(u)
		for _, n := range names {
			if store.IsActive(n) {
				continue // never prune an in-progress recording
			}
			p := centralPath(u, n)
			h, _ := store.Header(p)
			if !match(h.Timestamp) {
				continue
			}
			var sz int64
			if fi, e := os.Stat(p); e == nil {
				sz = fi.Size()
			}
			hits = append(hits, pruneTarget{u, n, p, sz})
			total += sz
		}
	}
	return hits, total
}

func printStorageOverview(users []string) {
	fmt.Printf("Central store: %s\n", store.CentralDir())
	fmt.Printf("  %-20s %9s %10s\n", "USER", "SESSIONS", "SIZE")
	var grandSize int64
	var grandN int
	for _, u := range users {
		names, _ := store.UserSessions(u)
		var sz int64
		for _, n := range names {
			if fi, e := os.Stat(centralPath(u, n)); e == nil {
				sz += fi.Size()
			}
		}
		grandSize += sz
		grandN += len(names)
		fmt.Printf("  %-20s %9d %10s\n", u, len(names), humanSize(sz))
	}
	fmt.Printf("  %-20s %9d %10s\n\n", "TOTAL", grandN, humanSize(grandSize))
}

func pruneHashPath() string { return filepath.Join(store.CentralDir(), ".prune.hash") }

// prunePasswordGate requires the prune password. On first use (no password set
// yet, e.g. just after install) it prompts to create one.
func prunePasswordGate(in *bufio.Reader) error {
	data, err := os.ReadFile(pruneHashPath())
	if os.IsNotExist(err) {
		fmt.Println("No prune password set yet — create one now (required to prune).")
		return setPrunePassword(in)
	}
	if err != nil {
		return err
	}
	if !verifyPassword(string(data), readPassword(in, "Prune password: ")) {
		return fmt.Errorf("incorrect prune password")
	}
	return nil
}

func setPrunePassword(in *bufio.Reader) error {
	p1 := readPassword(in, "New prune password: ")
	if len(p1) < 4 {
		return fmt.Errorf("password too short (min 4 chars)")
	}
	if readPassword(in, "Confirm password: ") != p1 {
		return fmt.Errorf("passwords do not match")
	}
	rec, err := hashPassword(p1)
	if err != nil {
		return err
	}
	if err := os.WriteFile(pruneHashPath(), []byte(rec), 0o600); err != nil {
		return err
	}
	fmt.Println("prune password set.")
	return nil
}

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := sha256.Sum256(append(salt, []byte(pw)...))
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(h[:]), nil
}

func verifyPassword(rec, pw string) bool {
	parts := strings.SplitN(strings.TrimSpace(rec), "$", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	h := sha256.Sum256(append(salt, []byte(pw)...))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(h[:])), []byte(parts[1])) == 1
}

func readPassword(in *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		if b, err := term.ReadPassword(fd); err == nil {
			fmt.Println()
			return strings.TrimSpace(string(b))
		}
	}
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}

func ask(in *bufio.Reader, prompt, def string) string {
	fmt.Printf("%s ", prompt)
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func humanSize(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for x := n / u; x >= u; x /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
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
	fs.SetOutput(io.Discard) // suppress flag's own usage; we print our own
	from := fs.String("from", "", "only sessions started at/after this time")
	to := fs.String("to", "", "only sessions started at/before this time")
	userFilter := fs.String("user", "", "limit to one user")
	ignore := fs.Bool("i", false, "case-insensitive match")
	all := fs.Bool("all", false, "list all sessions (no pattern needed)")
	if err := fs.Parse(args); err != nil {
		return searchUsage(err)
	}
	if fs.NArg() < 1 && !*all {
		return searchUsage(nil)
	}
	pattern := ""
	if fs.NArg() >= 1 {
		pattern = fs.Arg(0)
	}
	needle := pattern
	if *ignore {
		needle = strings.ToLower(needle)
	}

	fromT, toT, err := parseRange(*from, *to)
	if err != nil {
		return err
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
		matched += searchUser(u, needle, *ignore, fromT, toT, *all)
	}
	if matched == 0 {
		if *all {
			fmt.Println("no recordings")
		} else {
			fmt.Printf("no matches for %q\n", pattern)
		}
	}
	return nil
}

func searchUsage(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttrack search: %v\n\n", err)
	}
	fmt.Fprint(os.Stderr, `usage:
  ttrack search [--from T] [--to T] [--user U] [-i] <pattern>
  ttrack search --all [--from T] [--to T] [--user U]      list all sessions

flags:
  --from <time>   only sessions started at/after  (YYYY-MM-DD[ HH:MM] or RFC3339)
  --to   <time>   only sessions started at/before
  --user <name>   limit to one user
  -i              case-insensitive match
  --all           list every recorded session (no pattern needed)

examples:
  ttrack search nginx                         find "nginx" in any recording
  ttrack search -i ERROR                      case-insensitive
  ttrack search --user alice sudo             alice's sessions containing "sudo"
  ttrack search --from 2026-05-01 --to 2026-05-20 deploy
  ttrack search --all                         list all recorded sessions
  ttrack search --all --user alice            list all of alice's sessions
`)
	if err != nil {
		return err
	}
	return fmt.Errorf("missing search pattern")
}

func parseRange(from, to string) (fromT, toT time.Time, err error) {
	if from != "" {
		if fromT, err = parseTime(from); err != nil {
			return fromT, toT, fmt.Errorf("bad --from: %w", err)
		}
	}
	if to != "" {
		if toT, err = parseTime(to); err != nil {
			return fromT, toT, fmt.Errorf("bad --to: %w", err)
		}
	}
	return fromT, toT, nil
}

func inWindow(ts int64, fromT, toT time.Time) bool {
	if ts <= 0 {
		return true
	}
	st := time.Unix(ts, 0)
	if !fromT.IsZero() && st.Before(fromT) {
		return false
	}
	if !toT.IsZero() && st.After(toT) {
		return false
	}
	return true
}

func searchUser(u, needle string, ignore bool, fromT, toT time.Time, all bool) int {
	names, _ := store.UserSessions(u)
	matched := 0
	for _, name := range names {
		path := centralPath(u, name)
		h, herr := store.Header(path)
		if herr != nil {
			continue
		}
		if !inWindow(h.Timestamp, fromT, toT) {
			continue
		}
		var snips []string
		if !all {
			cmdMatch, s := scanCast(path, needle, ignore, h)
			if !cmdMatch && len(s) == 0 {
				continue
			}
			snips = s
		}
		matched++
		fmt.Printf("user=%s  when=%s  session=%s\n",
			u, store.Started(h), strings.TrimSuffix(name, ".cast"))
		fmt.Printf("    cmd: %s\n", clean(h.Command))
		for _, s := range snips {
			fmt.Printf("    > %s\n", s)
		}
	}
	return matched
}

// scanCast reports whether the recorded command matched, plus up to a few
// cleaned output snippet lines containing needle.
func scanCast(path, needle string, ignore bool, h cast.Header) (cmdMatch bool, snips []string) {
	hay := h.Command
	if ignore {
		hay = strings.ToLower(hay)
	}
	cmdMatch = needle != "" && strings.Contains(hay, needle)

	rc, err := store.OpenCast(path)
	if err != nil {
		return cmdMatch, nil
	}
	defer rc.Close()
	r := bufio.NewReader(rc)
	if _, err := cast.ReadHeader(r); err != nil {
		return cmdMatch, nil
	}
	return cmdMatch, scanOutput(r, needle, ignore)
}

func scanOutput(r *bufio.Reader, needle string, ignore bool) []string {
	const maxSnip = 5
	var snips []string
	for len(snips) < maxSnip {
		ev, err := cast.ReadEvent(r)
		if err != nil {
			break
		}
		if ev.Type == "o" {
			snips = appendMatches(snips, ev.Data, needle, ignore, maxSnip)
		}
	}
	return snips
}

func appendMatches(snips []string, data, needle string, ignore bool, max int) []string {
	for _, line := range strings.Split(data, "\n") {
		h := line
		if ignore {
			h = strings.ToLower(h)
		}
		if !strings.Contains(h, needle) {
			continue
		}
		if c := clean(line); c != "" {
			snips = append(snips, c)
		}
		if len(snips) >= max {
			break
		}
	}
	return snips
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
