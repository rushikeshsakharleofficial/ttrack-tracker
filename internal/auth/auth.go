// Package auth manages the optional ttrack playback password.
// The password hash is stored in /etc/ttrack/.playback_passwd (root:root 0600).
// When the file exists all `ttrack play` invocations prompt for the password.
package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

// PasswdFile is the path to the bcrypt hash file.
const PasswdFile = "/etc/ttrack/.playback_passwd"

// bcryptCost is the work factor used when hashing new passwords.
const bcryptCost = 12

// MaxAttempts is the number of wrong-password tries before PromptAndVerify fails.
const MaxAttempts = 3

// IsSet reports whether a playback password has been configured.
func IsSet() bool {
	_, err := os.Stat(PasswdFile)
	return err == nil
}

// SetPassword hashes password with bcrypt and writes it to PasswdFile.
// The file is created with mode 0600 under /etc/ttrack/ (must exist).
func SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("bcrypt: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(PasswdFile), 0700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(PasswdFile, hash, 0600); err != nil {
		return fmt.Errorf("write passwd: %w", err)
	}
	return nil
}

// Verify checks password against the stored hash.
// Returns nil on match, a non-nil error on mismatch or I/O error.
func Verify(password string) error {
	hash, err := os.ReadFile(PasswdFile)
	if err != nil {
		return fmt.Errorf("read passwd: %w", err)
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(password))
}

// Remove deletes the password file, disabling playback protection.
func Remove() error {
	err := os.Remove(PasswdFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// PromptAndVerify prompts for the playback password (no echo) up to MaxAttempts
// times. Returns nil if correct or if no password is set. Returns an error after
// MaxAttempts failures.
func PromptAndVerify() error {
	if !IsSet() {
		return nil
	}
	for i := 0; i < MaxAttempts; i++ {
		pw, err := ReadPassword("Playback password: ")
		if err != nil {
			return err
		}
		if err := Verify(pw); err == nil {
			return nil
		}
		if i < MaxAttempts-1 {
			fmt.Fprintln(os.Stderr, "ttrack: incorrect password, try again")
		}
	}
	return errors.New("incorrect playback password")
}

// ReadPassword reads a password from the terminal without echo.
func ReadPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(pw), "\r\n"), nil
}
