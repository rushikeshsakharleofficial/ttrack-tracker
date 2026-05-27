package ansible

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ttrack/internal/store"
)

// statusIcon returns a one-character status indicator with ANSI colour.
func statusIcon(s string) string {
	switch s {
	case "ok":
		return "\x1b[32m✓\x1b[0m" // green
	case "changed":
		return "\x1b[33m~\x1b[0m" // yellow
	case "failed":
		return "\x1b[31m✗\x1b[0m" // red
	case "unreachable":
		return "\x1b[35m!\x1b[0m" // magenta
	case "skipped":
		return "\x1b[90m-\x1b[0m" // dark grey
	default:
		return "?"
	}
}

// humanDur formats a duration into a human-readable string.
func humanDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// ansibleDir returns the ansible sub-directory inside the user's central dir.
func ansibleDir(user string) string {
	return filepath.Join(store.CentralDir(), user, "ansible")
}

// localAnsibleDir returns the user-local ansible dir (fail-open fallback path).
func localAnsibleDir() string {
	return filepath.Join(store.Dir(), "ansible")
}

// scanDir returns .ajsonl run-ids from dir, newest first.
func scanDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for i := len(entries) - 1; i >= 0; i-- {
		name := entries[i].Name()
		if strings.HasSuffix(name, ".ajsonl") {
			ids = append(ids, strings.TrimSuffix(name, ".ajsonl"))
		}
	}
	return ids, nil
}

// listRuns returns all .ajsonl run-ids for a user from the central store.
func listRuns(user string) ([]string, error) { return scanDir(ansibleDir(user)) }

// openRun opens and decrypts (if needed) a central-store .ajsonl file.
func openRun(user, id string) (*Run, error) {
	path := filepath.Join(ansibleDir(user), id+".ajsonl")
	rc, err := store.OpenCast(path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return ParseRun(rc)
}

// openLocalRun opens an .ajsonl from the user-local fallback dir (no decrypt).
func openLocalRun(id string) (*Run, error) {
	path := filepath.Join(localAnsibleDir(), id+".ajsonl")
	rc, err := store.OpenCast(path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return ParseRun(rc)
}

// List implements `ttrack ansible list [--user U]`.
func List(args []string) error {
	fs := flag.NewFlagSet("ansible list", flag.ContinueOnError)
	userFlag := fs.String("user", "", "limit to one user")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// localOnly: true when central store inaccessible (not root / not installed)
	localOnly := false
	var users []string
	if *userFlag != "" {
		users = []string{*userFlag}
	} else {
		u, err := store.Users()
		if err != nil {
			if os.IsPermission(err) || os.IsNotExist(err) {
				localOnly = true
			} else {
				return err
			}
		} else {
			users = u
		}
	}

	fmt.Printf("%-28s  %-20s  %-10s  %-6s %-6s %-6s  %-19s  %s\n",
		"RUN", "PLAYBOOK", "CONTROLLER", "OK", "CHG", "FAIL", "STARTED", "HOSTS")

	// Local fallback: show runs from ~/.local/share/ttrack/ansible/.
	if localOnly {
		ids, _ := scanDir(localAnsibleDir())
		if len(ids) == 0 {
			fmt.Printf("no ansible runs in %s\n", localAnsibleDir())
			return nil
		}
		for _, id := range ids {
			run, err := openLocalRun(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ttrack: %s: %v\n", id, err)
				continue
			}
			printRunRow(run, id)
		}
		return nil
	}

	for _, u := range users {
		ids, err := listRuns(u)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttrack: %s: %v\n", u, err)
			continue
		}
		for _, id := range ids {
			run, err := openRun(u, id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ttrack: %s/%s: %v\n", u, id, err)
				continue
			}
			printRunRow(run, id)
		}
	}
	return nil
}

// printRunRow prints one run as a table row.
func printRunRow(run *Run, id string) {
	started := "?"
	if !run.Started.IsZero() {
		started = run.Started.Format("2006-01-02 15:04:05")
	}
	playbook := run.Playbook
	if len(playbook) > 20 {
		playbook = "…" + playbook[len(playbook)-19:]
	}
	ctrl := run.Controller
	if len(ctrl) > 10 {
		ctrl = ctrl[:9] + "…"
	}
	hosts := strings.Join(run.Hosts, ",")
	if len(hosts) > 20 {
		hosts = hosts[:19] + "…"
	}
	fmt.Printf("%-28s  %-20s  %-10s  %-6d %-6d %-6d  %-19s  %s\n",
		id, playbook, ctrl,
		run.TotalOK, run.TotalChanged, run.TotalFailed,
		started, hosts)
}

// Show implements `ttrack ansible show <runid>`.
func Show(args []string) error {
	fs := flag.NewFlagSet("ansible show", flag.ContinueOnError)
	userFlag := fs.String("user", "", "user owning the run (default: search all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: ttrack ansible show [--user U] <runid>")
	}
	runID := fs.Arg(0)

	// Find the run: search all users unless --user is specified.
	var run *Run
	var foundUser string
	if *userFlag != "" {
		r, err := openRun(*userFlag, runID)
		if err != nil {
			return fmt.Errorf("cannot open run %s for user %s: %w", runID, *userFlag, err)
		}
		run = r
		foundUser = *userFlag
	} else {
		users, err := store.Users()
		if err == nil {
			for _, u := range users {
				r, err := openRun(u, runID)
				if err == nil {
					run = r
					foundUser = u
					break
				}
			}
		}
		// central store inaccessible or run not found — fall through to local
	}
	// Fall back to local dir when not found (or not accessible) in central store.
	if run == nil {
		if r, err := openLocalRun(runID); err == nil {
			run = r
			foundUser = "(local)"
		}
	}
	if run == nil {
		return fmt.Errorf("run %s not found in central store or local dir", runID)
	}

	// Header.
	started := "?"
	if !run.Started.IsZero() {
		started = run.Started.Format("2006-01-02 15:04:05")
	}
	fmt.Printf("Playbook : %s\n", run.Playbook)
	fmt.Printf("Run ID   : %s\n", run.ID)
	fmt.Printf("User     : %s\n", foundUser)
	fmt.Printf("Controller: %s\n", run.Controller)
	fmt.Printf("Started  : %s\n", started)
	fmt.Printf("Duration : %s\n", humanDur(run.Duration()))
	fmt.Printf("Hosts    : %s\n", strings.Join(run.Hosts, ", "))
	fmt.Println()

	// Tasks grouped by play.
	currentPlay := ""
	for _, t := range run.Tasks {
		if t.Play != currentPlay {
			currentPlay = t.Play
			fmt.Printf("\x1b[1mPLAY [%s]\x1b[0m\n", currentPlay)
		}
		icon := statusIcon(t.Status)
		mod := t.Module
		if mod != "" {
			mod = "(" + mod + ")"
		}
		ts := ""
		if t.T > 0 {
			ts = fmt.Sprintf(" @%s", time.Unix(int64(t.T), 0).Format("15:04:05"))
		}
		fmt.Printf("  %s %-12s  %-30s %s%s\n", icon, t.Host, t.Name, mod, ts)
		if t.Status == "failed" || t.Status == "unreachable" || t.Status == "changed" {
			if t.Stdout != "" {
				fmt.Printf("      stdout: %s\n", indentOutput(t.Stdout))
			}
			if t.Stderr != "" {
				fmt.Printf("      stderr: %s\n", indentOutput(t.Stderr))
			}
			if t.RC != 0 {
				fmt.Printf("      rc: %d\n", t.RC)
			}
		}
	}

	// Recap.
	if len(run.Stats) > 0 {
		fmt.Println()
		fmt.Println("\x1b[1mPLAY RECAP\x1b[0m")
		for _, h := range run.Hosts {
			s, ok := run.Stats[h]
			if !ok {
				continue
			}
			fmt.Printf("  %-20s ok=%-4d changed=%-4d failed=%-4d unreachable=%-4d skipped=%d\n",
				h, s.OK, s.Changed, s.Failed, s.Unreachable, s.Skipped)
		}
	}
	return nil
}

// Dispatch handles `ttrack ansible <subcommand> [args...]`.
func Dispatch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ttrack ansible <list|show> [args...]")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "list":
		return List(rest)
	case "show":
		return Show(rest)
	default:
		return fmt.Errorf("ansible: unknown subcommand %q (list|show)", sub)
	}
}

// indentOutput indents multi-line output for display inside a task block.
func indentOutput(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 1 {
		return lines[0]
	}
	var b strings.Builder
	b.WriteString(lines[0])
	for _, l := range lines[1:] {
		b.WriteString("\n             ")
		b.WriteString(l)
	}
	return b.String()
}
