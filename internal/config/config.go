// Package config loads ttrack runtime configuration from /etc/ttrack/ttrack.conf
// (overridable by TTRACK_CONFIG env var). All keys have safe built-in defaults
// so the file is optional. Environment variables always override file values.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultPath is the canonical config file location.
const DefaultPath = "/etc/ttrack/ttrack.conf"

// Config holds all tuneable runtime settings.
type Config struct {
	// SocketPath is the Unix socket the ttrackd daemon listens on.
	SocketPath string
	// CentralDir is the root of the root-only central session store.
	CentralDir string
	// KeyFile is the path to the at-rest encryption key (absolute or relative
	// to CentralDir when it doesn't start with "/").
	KeyFile string
	// DialTimeout is how long ttrack rec waits when connecting to ttrackd.
	DialTimeout time.Duration
	// EOFGrace is the delay between sending Ctrl-D and force-closing the PTY
	// master when stdin reaches EOF in a non-interactive session.
	EOFGrace time.Duration
	// AnsibleOutputCap is the maximum bytes stored per ansible task output.
	AnsibleOutputCap int
}

// defaults returns a Config populated with factory defaults.
func defaults() Config {
	return Config{
		SocketPath:       "/run/ttrackd.sock",
		CentralDir:       "/var/lib/ttrack",
		KeyFile:          "", // resolved to CentralDir/.ttrack.key when empty
		DialTimeout:      1 * time.Second,
		EOFGrace:         500 * time.Millisecond,
		AnsibleOutputCap: 8 * 1024,
	}
}

// ResolvedKeyFile returns the absolute path to the encryption key, resolving
// a relative KeyFile against CentralDir.
func (c *Config) ResolvedKeyFile() string {
	if c.KeyFile == "" {
		return filepath.Join(c.CentralDir, ".ttrack.key")
	}
	if filepath.IsAbs(c.KeyFile) {
		return c.KeyFile
	}
	return filepath.Join(c.CentralDir, c.KeyFile)
}

var (
	once   sync.Once
	global *Config
)

// Load returns the process-wide config singleton, parsed on first call.
// The config file path is taken from TTRACK_CONFIG env var, falling back to
// DefaultPath. A missing or unreadable file is silently ignored (defaults used).
func Load() *Config {
	once.Do(func() {
		cfg := defaults()
		path := os.Getenv("TTRACK_CONFIG")
		if path == "" {
			path = DefaultPath
		}
		_ = parseFile(path, &cfg)
		applyEnv(&cfg)
		global = &cfg
	})
	return global
}

// Reset clears the singleton so the next Load() re-reads the config. Intended
// for use in tests only.
func Reset() {
	once = sync.Once{}
	global = nil
}

// parseFile reads key=value pairs from path into cfg. Unknown keys and parse
// errors for individual values are silently skipped so a partial file still
// applies its valid entries.
func parseFile(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Strip inline comments.
		if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		applyKey(cfg, k, v)
	}
	return sc.Err()
}

// applyKey applies a single parsed key=value to cfg. Unknown keys are ignored.
func applyKey(cfg *Config, k, v string) {
	switch k {
	case "socket_path":
		if v != "" {
			cfg.SocketPath = v
		}
	case "central_dir":
		if v != "" {
			cfg.CentralDir = v
		}
	case "key_file":
		cfg.KeyFile = v // empty string is valid (means "use default relative path")
	case "dial_timeout_sec":
		if s, err := strconv.ParseFloat(v, 64); err == nil && s > 0 {
			cfg.DialTimeout = time.Duration(s * float64(time.Second))
		}
	case "eof_grace_ms":
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms >= 0 {
			cfg.EOFGrace = time.Duration(ms) * time.Millisecond
		}
	case "ansible_output_cap":
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.AnsibleOutputCap = n
		}
	}
}

// applyEnv overrides cfg fields from environment variables. Env vars always
// win over the config file.
func applyEnv(cfg *Config) {
	if v := os.Getenv("TTRACKD_SOCK"); v != "" {
		cfg.SocketPath = v
	}
	if v := os.Getenv("TTRACK_CENTRAL_DIR"); v != "" {
		cfg.CentralDir = v
	}
	if v := os.Getenv("TTRACK_KEY_FILE"); v != "" {
		cfg.KeyFile = v
	}
	if v := os.Getenv("TTRACK_DIAL_TIMEOUT_SEC"); v != "" {
		if s, err := strconv.ParseFloat(v, 64); err == nil && s > 0 {
			cfg.DialTimeout = time.Duration(s * float64(time.Second))
		}
	}
	if v := os.Getenv("TTRACK_EOF_GRACE_MS"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms >= 0 {
			cfg.EOFGrace = time.Duration(ms) * time.Millisecond
		}
	}
	if v := os.Getenv("TTRACK_ANSIBLE_OUTPUT_CAP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.AnsibleOutputCap = n
		}
	}
}
