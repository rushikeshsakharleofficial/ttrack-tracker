// Package backup runs periodic and on-demand backups of the central recording
// store to a remote target. All transfer is delegated to an external command
// (aws, gsutil, rsync) — no new SDK dependencies are introduced.
package backup

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"ttrack/internal/config"
)

// runCommand is the shell-out function. Tests replace it with a spy to verify
// the correct command is assembled without performing actual transfers.
var runCommand = func(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Omit args from error to avoid leaking credentials in S3/GCS URLs.
		return fmt.Errorf("%s: command failed: %w\noutput: %s", name, err, redactOutput(out))
	}
	return nil
}

// redactOutput trims combined output and removes lines that look like credentials.
func redactOutput(out []byte) string {
	const maxLen = 512
	s := strings.TrimSpace(string(out))
	if len(s) > maxLen {
		s = s[:maxLen] + "...[truncated]"
	}
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if credentialLike(line) {
			lines = append(lines, "[redacted]")
		} else {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func credentialLike(line string) bool {
	lower := strings.ToLower(line)
	for _, kw := range []string{"password", "token", "secret", "key=", "auth=", "aws_secret", "credential"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// validateTarget checks that backup_target is appropriate for the given type.
func validateTarget(backupType, target string) error {
	switch backupType {
	case "bucket_aws":
		if !strings.HasPrefix(target, "s3://") {
			return fmt.Errorf("backup_target for bucket_aws must start with s3:// (got %q)", target)
		}
	case "bucket_gcp":
		if !strings.HasPrefix(target, "gs://") {
			return fmt.Errorf("backup_target for bucket_gcp must start with gs:// (got %q)", target)
		}
	case "rsync":
		if !strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "//") && !strings.Contains(target, ":") {
			return fmt.Errorf("backup_target for rsync must be an absolute path or remote host:path (got %q)", target)
		}
	}
	return nil
}

// buildBackupArgs returns the binary name and argument list for the configured
// backup type. Pure — no I/O — so it can be tested without injection.
func buildBackupArgs(cfg *config.Config) (name string, args []string, err error) {
	if cfg.BackupType == "" {
		return "", nil, errors.New("backup disabled: backup_type is not set")
	}
	if cfg.BackupTarget == "" {
		return "", nil, errors.New("backup misconfigured: backup_target is empty")
	}
	if err := validateTarget(cfg.BackupType, cfg.BackupTarget); err != nil {
		return "", nil, err
	}
	switch cfg.BackupType {
	case "bucket_aws":
		return "aws", []string{"s3", "sync", cfg.CentralDir, cfg.BackupTarget}, nil
	case "bucket_gcp":
		return "gsutil", []string{"-m", "rsync", "-r", cfg.CentralDir, cfg.BackupTarget}, nil
	case "rsync":
		// Trailing slash copies directory contents, not a nested sub-directory.
		return "rsync", []string{"-a", "--delete", cfg.CentralDir + "/", cfg.BackupTarget}, nil
	default:
		return "", nil, fmt.Errorf("backup: unknown type %q", cfg.BackupType)
	}
}

// Run executes a backup for the given configuration. ctx cancellation kills
// the external process. Returns an error when the type is disabled,
// misconfigured, or the external command fails.
func Run(ctx context.Context, cfg *config.Config) error {
	name, args, err := buildBackupArgs(cfg)
	if err != nil {
		return err
	}
	return runCommand(ctx, name, args...)
}

// RunCLI is the entry point for `ttrack backup`. Runs a backup immediately and
// prints a status line. Access enforced by filesystem permissions (CentralDir
// is root:root 0700).
func RunCLI(_ []string) error {
	cfg := config.Load()
	if cfg.BackupType == "" {
		return errors.New("backup is not configured (backup_type is empty in config)")
	}
	if err := Run(context.Background(), cfg); err != nil {
		return err
	}
	fmt.Printf("backup completed (type=%s target=%s)\n", cfg.BackupType, cfg.BackupTarget)
	return nil
}
