package backup

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ttrack/internal/config"
)

func TestBuildBackupArgs_AWS(t *testing.T) {
	cfg := &config.Config{
		BackupType:   "bucket_aws",
		BackupTarget: "s3://my-bucket/ttrack",
		CentralDir:   "/var/lib/ttrack",
	}
	name, args, err := buildBackupArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "aws" {
		t.Errorf("name = %q, want aws", name)
	}
	want := []string{"s3", "sync", "/var/lib/ttrack", "s3://my-bucket/ttrack"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestBuildBackupArgs_GCP(t *testing.T) {
	cfg := &config.Config{
		BackupType:   "bucket_gcp",
		BackupTarget: "gs://my-bucket/ttrack",
		CentralDir:   "/var/lib/ttrack",
	}
	name, args, err := buildBackupArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gsutil" {
		t.Errorf("name = %q, want gsutil", name)
	}
	want := []string{"-m", "rsync", "-r", "/var/lib/ttrack", "gs://my-bucket/ttrack"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestBuildBackupArgs_Rsync(t *testing.T) {
	cfg := &config.Config{
		BackupType:   "rsync",
		BackupTarget: "user@host:/backups/ttrack",
		CentralDir:   "/var/lib/ttrack",
	}
	name, args, err := buildBackupArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "rsync" {
		t.Errorf("name = %q, want rsync", name)
	}
	// Trailing slash on CentralDir is required for rsync to copy contents,
	// not create a nested sub-directory.
	want := []string{"-a", "--delete", "/var/lib/ttrack/", "user@host:/backups/ttrack"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestBuildBackupArgs_DisabledTypeReturnsError(t *testing.T) {
	cfg := &config.Config{BackupType: "", BackupTarget: "s3://bucket", CentralDir: "/var/lib/ttrack"}
	_, _, err := buildBackupArgs(cfg)
	if err == nil {
		t.Fatal("expected error for empty backup_type, got nil")
	}
}

func TestBuildBackupArgs_EmptyTargetReturnsError(t *testing.T) {
	cfg := &config.Config{BackupType: "bucket_aws", BackupTarget: "", CentralDir: "/var/lib/ttrack"}
	_, _, err := buildBackupArgs(cfg)
	if err == nil {
		t.Fatal("expected error for empty backup_target, got nil")
	}
}

func TestRunInvokesCorrectCommand(t *testing.T) {
	var gotName string
	var gotArgs []string
	orig := runCommand
	runCommand = func(_ context.Context, name string, args ...string) error {
		gotName = name
		gotArgs = args
		return nil
	}
	defer func() { runCommand = orig }()

	cfg := &config.Config{
		BackupType:   "rsync",
		BackupTarget: "user@host:/backup",
		CentralDir:   "/var/lib/ttrack",
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotName != "rsync" {
		t.Errorf("command = %q, want rsync", gotName)
	}
	wantArgs := []string{"-a", "--delete", "/var/lib/ttrack/", "user@host:/backup"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestRunDisabledReturnsError(t *testing.T) {
	cfg := &config.Config{BackupType: "", CentralDir: "/var/lib/ttrack"}
	if err := Run(context.Background(), cfg); err == nil {
		t.Fatal("expected error for disabled backup, got nil")
	}
}

func TestRunCommandFailurePropagates(t *testing.T) {
	orig := runCommand
	runCommand = func(_ context.Context, name string, args ...string) error {
		return errors.New("simulated transfer failure")
	}
	defer func() { runCommand = orig }()

	cfg := &config.Config{
		BackupType:   "bucket_aws",
		BackupTarget: "s3://bucket",
		CentralDir:   "/var/lib/ttrack",
	}
	if err := Run(context.Background(), cfg); err == nil {
		t.Fatal("expected error to propagate from runCommand, got nil")
	}
}

func TestRunCancelledContextKillsProcess(t *testing.T) {
	orig := runCommand
	defer func() { runCommand = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	called := false
	runCommand = func(_ context.Context, name string, args ...string) error {
		called = true
		return ctx.Err()
	}

	cfg := &config.Config{
		BackupType:   "rsync",
		BackupTarget: "user@host:/path",
		CentralDir:   "/tmp",
	}
	err := Run(ctx, cfg)
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
	if !called {
		t.Error("runCommand spy was not called")
	}
}
