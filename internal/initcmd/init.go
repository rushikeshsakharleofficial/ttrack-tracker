// Package initcmd implements `ttrack init` — the first-time setup wizard and
// playback-password management commands.
package initcmd

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"ttrack/internal/auth"
	"ttrack/internal/config"
)

// Run dispatches the init subcommand.
func Run(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	resetPw := fs.Bool("reset-password", false, "change the playback password (requires current password)")
	clearPw := fs.Bool("clear-password", false, "remove playback password protection (requires current password)")
	enableSSH := fs.Bool("enable-ssh-forcecommand", false, "install sshd ForceCommand drop-in for non-interactive SSH recording (root)")
	disableSSH := fs.Bool("disable-ssh-forcecommand", false, "remove sshd ForceCommand drop-in (root)")
	enableAuto := fs.Bool("enable-autorec", false, "install interactive login auto-record hook in /etc/profile.d (root)")
	disableAuto := fs.Bool("disable-autorec", false, "remove interactive login auto-record hook from /etc/profile.d (root)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch {
	case *resetPw:
		return cmdResetPassword()
	case *clearPw:
		return cmdClearPassword()
	case *enableSSH:
		return cmdSSHForceCommand(true)
	case *disableSSH:
		return cmdSSHForceCommand(false)
	case *enableAuto:
		return cmdAutoRec(true)
	case *disableAuto:
		return cmdAutoRec(false)
	default:
		return wizard()
	}
}

// ─── wizard ───────────────────────────────────────────────────────────────────

func wizard() error {
	fmt.Fprintln(os.Stderr, "ttrack initialization")
	fmt.Fprintln(os.Stderr, "═══════════════════════════════════════")
	fmt.Fprintln(os.Stderr)

	cfg := config.Load()
	ok := true

	// [1/4] Config file
	cfgPath := os.Getenv("TTRACK_CONFIG")
	if cfgPath == "" {
		cfgPath = config.DefaultPath
	}
	if _, err := os.Stat(cfgPath); err == nil {
		printStep(1, 4, "Config ("+cfgPath+")", "OK")
	} else {
		printStep(1, 4, "Config ("+cfgPath+")", "MISSING — defaults in use")
	}

	// [2/4] Daemon socket
	conn, derr := net.DialTimeout("unix", cfg.SocketPath, time.Second)
	if derr == nil {
		conn.Close()
		printStep(2, 4, "Daemon (ttrackd) on "+cfg.SocketPath, "RUNNING")
	} else {
		printStep(2, 4, "Daemon (ttrackd) on "+cfg.SocketPath, "NOT RUNNING — start with: systemctl start ttrackd")
		ok = false
	}

	// [3/4] Encryption key
	keyPath := cfg.ResolvedKeyFile()
	if _, kerr := os.Stat(keyPath); kerr == nil {
		printStep(3, 4, "Encryption key ("+keyPath+")", "OK")
	} else if os.IsPermission(kerr) {
		// Key exists but not readable by this user (root-only file). Not an error.
		printStep(3, 4, "Encryption key ("+keyPath+")", "OK (root-only — run as root to verify)")
	} else {
		printStep(3, 4, "Encryption key ("+keyPath+")", "MISSING — start ttrackd to generate")
		ok = false
	}

	// [4/4] Playback password
	if auth.IsSet() {
		printStep(4, 4, "Playback password", "SET")
		fmt.Fprintln(os.Stderr)
		if ok {
			fmt.Fprintln(os.Stderr, "All good. Use --reset-password or --clear-password to change.")
		}
	} else {
		printStep(4, 4, "Playback password", "not set")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Any root user can replay sessions without a password.")
		fmt.Fprint(os.Stderr, "  Set a playback password? [y/N] ")
		var ans string
		fmt.Fscanln(os.Stdin, &ans)
		if ans == "y" || ans == "Y" || ans == "yes" {
			if err := promptSetNewPassword(); err != nil {
				return err
			}
		}
	}

	fmt.Fprintln(os.Stderr)
	if ok {
		fmt.Fprintln(os.Stderr, "ttrack init complete. Run 'ttrack --check' to verify config values.")
	} else {
		fmt.Fprintln(os.Stderr, "ttrack init complete with warnings. Resolve the issues above.")
	}
	return nil
}

// ─── password subcommands ─────────────────────────────────────────────────────

func cmdResetPassword() error {
	if !auth.IsSet() {
		// No existing password — allow setting one directly.
		fmt.Fprintln(os.Stderr, "No playback password set. Setting a new one.")
		return promptSetNewPassword()
	}
	if err := verifyCurrentPassword(); err != nil {
		return err
	}
	return promptSetNewPassword()
}

func cmdClearPassword() error {
	if !auth.IsSet() {
		fmt.Fprintln(os.Stderr, "No playback password set — nothing to clear.")
		return nil
	}
	if err := verifyCurrentPassword(); err != nil {
		return err
	}
	if err := auth.Remove(); err != nil {
		return fmt.Errorf("remove password: %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ Playback password removed. Sessions can be replayed without a password.")
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func verifyCurrentPassword() error {
	for i := 0; i < auth.MaxAttempts; i++ {
		pw, err := auth.ReadPassword("Current password: ")
		if err != nil {
			return err
		}
		if err := auth.Verify(pw); err == nil {
			return nil
		}
		if i < auth.MaxAttempts-1 {
			fmt.Fprintln(os.Stderr, "ttrack: incorrect password, try again")
		}
	}
	return errors.New("incorrect password — aborting")
}

func promptSetNewPassword() error {
	for {
		pw, err := auth.ReadPassword("New password:     ")
		if err != nil {
			return err
		}
		if len(pw) < 8 {
			fmt.Fprintln(os.Stderr, "ttrack: password must be at least 8 characters")
			continue
		}
		confirm, err := auth.ReadPassword("Confirm password: ")
		if err != nil {
			return err
		}
		if pw != confirm {
			fmt.Fprintln(os.Stderr, "ttrack: passwords do not match, try again")
			continue
		}
		if err := auth.SetPassword(pw); err != nil {
			return fmt.Errorf("set password: %w", err)
		}
		fmt.Fprintln(os.Stderr, "✓ Playback password set.")
		return nil
	}
}

func printStep(n, total int, label, status string) {
	fmt.Fprintf(os.Stderr, "[%d/%d] %-44s %s\n", n, total, label, status)
}

const (
	sshdDropinDir  = "/etc/ssh/sshd_config.d"
	sshdDropinFile = "/etc/ssh/sshd_config.d/zz-ttrack.conf"
	sshdDropinBody = "# Installed by ttrack init --enable-ssh-forcecommand.\n" +
		"# Remove with: sudo ttrack init --disable-ssh-forcecommand\n" +
		"# The wrapper is fail-open: scp/sftp/rsync pass through untouched.\n" +
		"ForceCommand /usr/libexec/ttrack-ssh-wrap\n"

	autorecDest = "/etc/profile.d/ttrack-autorec.sh"
	autorecSrc  = "/usr/share/doc/ttrack/ttrack-autorec.sh.example"
)

func cmdSSHForceCommand(enable bool) error {
	if os.Getuid() != 0 {
		return errors.New("--enable/disable-ssh-forcecommand requires root")
	}
	if !enable {
		if err := os.Remove(sshdDropinFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", sshdDropinFile, err)
		}
		fmt.Fprintln(os.Stderr, "ttrack: SSH ForceCommand removed. Reload sshd to apply.")
		return nil
	}
	if _, err := os.Stat(sshdDropinDir); os.IsNotExist(err) {
		return fmt.Errorf("%s does not exist — sshd on this system may not support drop-ins", sshdDropinDir)
	}
	if err := os.WriteFile(sshdDropinFile, []byte(sshdDropinBody), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", sshdDropinFile, err)
	}
	fmt.Fprintln(os.Stderr, "ttrack: SSH ForceCommand installed. Reload sshd to apply:")
	fmt.Fprintln(os.Stderr, "  sudo systemctl reload ssh || sudo systemctl reload sshd")
	fmt.Fprintln(os.Stderr, "Disable with: sudo ttrack init --disable-ssh-forcecommand")
	return nil
}

func cmdAutoRec(enable bool) error {
	if os.Getuid() != 0 {
		return errors.New("--enable/disable-autorec requires root")
	}
	if !enable {
		if err := os.Remove(autorecDest); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", autorecDest, err)
		}
		fmt.Fprintln(os.Stderr, "ttrack: interactive auto-record disabled.")
		return nil
	}
	src, err := os.ReadFile(autorecSrc)
	if err != nil {
		return fmt.Errorf("read example hook %s: %w", autorecSrc, err)
	}
	if err := os.WriteFile(autorecDest, src, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", autorecDest, err)
	}
	fmt.Fprintln(os.Stderr, "ttrack: interactive auto-record enabled (new login shells only).")
	fmt.Fprintln(os.Stderr, "Disable with: sudo ttrack init --disable-autorec")
	return nil
}
