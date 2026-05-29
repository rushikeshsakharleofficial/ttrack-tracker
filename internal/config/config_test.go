package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(Reset)
	Reset()
}

func TestDefaults(t *testing.T) {
	resetForTest(t)
	// Point at a nonexistent file so defaults are used.
	t.Setenv("TTRACK_CONFIG", "/nonexistent/ttrack.conf")

	cfg := Load()
	if cfg.SocketPath != "/run/ttrackd.sock" {
		t.Errorf("SocketPath = %q, want /run/ttrackd.sock", cfg.SocketPath)
	}
	if cfg.CentralDir != "/var/lib/ttrack" {
		t.Errorf("CentralDir = %q, want /var/lib/ttrack", cfg.CentralDir)
	}
	if cfg.DialTimeout != 1*time.Second {
		t.Errorf("DialTimeout = %v, want 1s", cfg.DialTimeout)
	}
	if cfg.EOFGrace != 500*time.Millisecond {
		t.Errorf("EOFGrace = %v, want 500ms", cfg.EOFGrace)
	}
	if cfg.AnsibleOutputCap != 8*1024 {
		t.Errorf("AnsibleOutputCap = %d, want 8192", cfg.AnsibleOutputCap)
	}
	if cfg.ScrollBuffer != 32*1024 {
		t.Errorf("ScrollBuffer = %d, want 32768", cfg.ScrollBuffer)
	}
	if cfg.LogLevel != 3 {
		t.Errorf("LogLevel = %d, want 3", cfg.LogLevel)
	}
	if cfg.LogFile != "/var/log/ttrack/ttrack.log" {
		t.Errorf("LogFile = %q, want /var/log/ttrack/ttrack.log", cfg.LogFile)
	}
	if cfg.SessionCap != 10 {
		t.Errorf("SessionCap = %d, want default 10", cfg.SessionCap)
	}
}

func TestSessionCapFromFile(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "ttrack.conf")
	os.WriteFile(f, []byte("session_cap = 25\n"), 0o644)
	t.Setenv("TTRACK_CONFIG", f)

	if cfg := Load(); cfg.SessionCap != 25 {
		t.Errorf("SessionCap = %d, want 25", cfg.SessionCap)
	}
}

func TestSessionCapEnvOverrides(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "ttrack.conf")
	os.WriteFile(f, []byte("session_cap = 25\n"), 0o644)
	t.Setenv("TTRACK_CONFIG", f)
	t.Setenv("TTRACK_SESSION_CAP", "7")

	if cfg := Load(); cfg.SessionCap != 7 {
		t.Errorf("SessionCap = %d, want 7 (env wins)", cfg.SessionCap)
	}
}

func TestSessionCapInvalidFallsToDefault(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "ttrack.conf")
	os.WriteFile(f, []byte("session_cap = 0\n"), 0o644)
	t.Setenv("TTRACK_CONFIG", f)

	if cfg := Load(); cfg.SessionCap != 10 {
		t.Errorf("SessionCap = %d, want default 10 for invalid value", cfg.SessionCap)
	}
}

// TestParseIsFreshAndDoesNotMutateSingleton verifies Parse() re-reads the
// current environment on every call and never touches the Load() singleton,
// so the daemon can reload config on SIGHUP without racing other goroutines
// that hold the shared *Config from Load().
func TestParseIsFreshAndDoesNotMutateSingleton(t *testing.T) {
	resetForTest(t)
	t.Setenv("TTRACK_CONFIG", "/nonexistent/ttrack.conf")
	t.Setenv("TTRACK_SESSION_CAP", "3")

	cached := Load()
	if cached.SessionCap != 3 {
		t.Fatalf("Load() SessionCap = %d, want 3", cached.SessionCap)
	}

	// Change the environment as if the operator edited config, then reload.
	os.Setenv("TTRACK_SESSION_CAP", "9")
	defer os.Unsetenv("TTRACK_SESSION_CAP")

	fresh := Parse()
	if fresh.SessionCap != 9 {
		t.Errorf("Parse() SessionCap = %d, want 9 (fresh re-read)", fresh.SessionCap)
	}
	// The cached singleton must be untouched.
	if cached.SessionCap != 3 {
		t.Errorf("Load() singleton mutated to %d; Parse() must not touch it", cached.SessionCap)
	}
	if fresh == cached {
		t.Error("Parse() returned the singleton pointer; want a fresh independent *Config")
	}
}

func TestResolvedKeyFile(t *testing.T) {
	tests := []struct {
		name       string
		centralDir string
		keyFile    string
		want       string
	}{
		{"empty key_file uses default", "/var/lib/ttrack", "", "/var/lib/ttrack/.ttrack.key"},
		{"absolute key_file used as-is", "/var/lib/ttrack", "/etc/ttrack/key", "/etc/ttrack/key"},
		{"relative key_file joined with central_dir", "/var/lib/ttrack", "keys/my.key", "/var/lib/ttrack/keys/my.key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{CentralDir: tc.centralDir, KeyFile: tc.keyFile}
			got := cfg.ResolvedKeyFile()
			if got != tc.want {
				t.Errorf("ResolvedKeyFile() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPartialFile(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "ttrack.conf")
	os.WriteFile(f, []byte("socket_path = /tmp/test.sock\n# comment\neof_grace_ms = 250\n"), 0o644)
	t.Setenv("TTRACK_CONFIG", f)

	cfg := Load()
	if cfg.SocketPath != "/tmp/test.sock" {
		t.Errorf("SocketPath = %q, want /tmp/test.sock", cfg.SocketPath)
	}
	if cfg.EOFGrace != 250*time.Millisecond {
		t.Errorf("EOFGrace = %v, want 250ms", cfg.EOFGrace)
	}
	// Unset keys keep defaults.
	if cfg.CentralDir != "/var/lib/ttrack" {
		t.Errorf("CentralDir = %q, want default", cfg.CentralDir)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "ttrack.conf")
	os.WriteFile(f, []byte("socket_path = /from/file.sock\nlog_file = /from/file.log\n"), 0o644)
	t.Setenv("TTRACK_CONFIG", f)
	t.Setenv("TTRACKD_SOCK", "/from/env.sock")
	t.Setenv("TTRACK_LOG_FILE", "/from/env.log")

	cfg := Load()
	if cfg.SocketPath != "/from/env.sock" {
		t.Errorf("SocketPath = %q, want /from/env.sock (env wins)", cfg.SocketPath)
	}
	if cfg.LogFile != "/from/env.log" {
		t.Errorf("LogFile = %q, want /from/env.log (env wins)", cfg.LogFile)
	}
}

func TestBadValuesFallToDefault(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "ttrack.conf")
	// All values are invalid — should silently keep defaults.
	os.WriteFile(f, []byte(
		"dial_timeout_sec = not-a-number\n"+
			"eof_grace_ms = -99\n"+
			"ansible_output_cap = 0\n"+
			"scroll_buffer = 100\n",
	), 0o644)
	t.Setenv("TTRACK_CONFIG", f)

	cfg := Load()
	if cfg.DialTimeout != 1*time.Second {
		t.Errorf("DialTimeout = %v, want default 1s", cfg.DialTimeout)
	}
	if cfg.EOFGrace != 500*time.Millisecond {
		t.Errorf("EOFGrace = %v, want default 500ms", cfg.EOFGrace)
	}
	if cfg.AnsibleOutputCap != 8*1024 {
		t.Errorf("AnsibleOutputCap = %d, want default 8192", cfg.AnsibleOutputCap)
	}
	if cfg.ScrollBuffer != 32*1024 {
		t.Errorf("ScrollBuffer = %d, want default 32768", cfg.ScrollBuffer)
	}
}

func TestInlineComment(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "ttrack.conf")
	os.WriteFile(f, []byte("socket_path = /tmp/sock.sock # this is a comment\n"), 0o644)
	t.Setenv("TTRACK_CONFIG", f)

	cfg := Load()
	if cfg.SocketPath != "/tmp/sock.sock" {
		t.Errorf("SocketPath = %q, want /tmp/sock.sock (inline comment stripped)", cfg.SocketPath)
	}
}

func TestSingleton(t *testing.T) {
	resetForTest(t)
	t.Setenv("TTRACK_CONFIG", "/nonexistent/ttrack.conf")
	a := Load()
	b := Load()
	if a != b {
		t.Error("Load() returned different pointers — singleton broken")
	}
}
