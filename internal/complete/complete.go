// Package complete provides shell completion: a hidden `__complete` helper that
// emits dynamic candidates, and `completion <shell>` that prints the script.
package complete

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ttrack/internal/store"
)

//go:embed ttrack.bash
var bashScript string

// Script handles `ttrack completion <shell>` — prints the completion script.
func Script(args []string) error {
	shell := "bash"
	if len(args) > 0 {
		shell = args[0]
	}
	switch shell {
	case "bash":
		fmt.Print(bashScript)
		return nil
	default:
		return fmt.Errorf("unsupported shell %q (only: bash)", shell)
	}
}

// Complete handles `ttrack __complete <kind>` — prints newline-separated
// candidates. Errors are swallowed (print nothing) so completion never noisily
// fails, e.g. when a non-root user cannot read the central store.
func Complete(args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "subcommands":
		fmt.Println("rec play ls ls-user play-user tail tree completion help")
	case "local-sessions":
		if names, err := castNames(store.Dir()); err == nil {
			fmt.Println(strings.Join(names, "\n"))
		}
	case "users":
		if users, err := store.Users(); err == nil {
			fmt.Println(strings.Join(users, "\n"))
		}
	case "central-sessions":
		users, err := store.Users()
		if err != nil {
			return nil
		}
		var ids []string
		for _, u := range users {
			names, _ := store.UserSessions(u)
			for _, n := range names {
				ids = append(ids, strings.TrimSuffix(n, ".cast"))
			}
		}
		fmt.Println(strings.Join(ids, "\n"))
	}
	return nil
}

func castNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".cast" {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
