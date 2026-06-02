// Package daemon implements ttrackd: the root collector that receives session
// recordings from per-user `ttrack rec` clients, stores them in the central
// root-only store, and fans out live sessions to root tailers.
//
// Protocol (line-based handshake, then raw bytes):
//
//	client -> "REC\n"              then streams asciinema v2 cast bytes (recorder)
//	client -> "TAIL <id>\n"        then reads live cast bytes (root only)
//	client -> "ANSIBLE <runid>\n"  then streams JSON-lines ansible run bytes
package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"ttrack/internal/ansible"
	"ttrack/internal/backup"
	"ttrack/internal/config"
	"ttrack/internal/crypto"
	"ttrack/internal/logger"
	"ttrack/internal/store"
)

// isTemporary returns true for transient network errors that are safe to retry.
// net.Error.Temporary() was deprecated in Go 1.18; we check syscall-level EAGAIN/EINTR.
func isTemporary(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	// EINTR or EAGAIN from accept — safe to retry.
	var errno interface{ Temporary() bool }
	if errors.As(err, &errno) {
		return errno.Temporary()
	}
	return false
}

// subChanCap is the number of chunks buffered per tailer before it is
// considered lagging and dropped. A chunk is one recorder Read (<=32KiB), so
// this absorbs a brief stall without unbounded memory growth.
const subChanCap = 256

// subscriber is a single live tailer. Its conn is written to by a dedicated
// drain goroutine reading from ch, so a slow/stuck conn never blocks the
// recorder or other tailers (the fan-out is decoupled from the disk write).
type subscriber struct {
	conn net.Conn
	ch   chan []byte
	done chan struct{} // closed by the drain goroutine when it exits
}

type session struct {
	mu   sync.Mutex
	f    *os.File  // underlying file, for sync/close
	enc  io.Writer // encrypting writer over f; recorded bytes land as ciphertext
	subs map[net.Conn]*subscriber
	done bool
}

// write persists b to disk synchronously and in order, then fans b out to every
// live tailer via a NON-BLOCKING send of a private copy. The caller may reuse b
// after write returns, so the copy is mandatory. A tailer whose channel is full
// (lagging) is dropped rather than blocking the recorder or peer tailers.
func (s *session) write(b []byte) error {
	s.mu.Lock()
	if s.enc != nil {
		if _, err := s.enc.Write(b); err != nil { // encrypted to disk
			s.mu.Unlock()
			return err
		}
	}
	if len(s.subs) == 0 {
		s.mu.Unlock()
		return nil
	}
	// Copy once; the same immutable slice is safe to share across subscribers.
	cp := make([]byte, len(b))
	copy(cp, b)
	var dropped []*subscriber
	for c, sub := range s.subs {
		select {
		case sub.ch <- cp:
		default:
			// Lagging tailer: drop it instead of blocking the disk write path.
			delete(s.subs, c)
			close(sub.ch)
			dropped = append(dropped, sub)
		}
	}
	s.mu.Unlock()
	// Close dropped conns outside the lock so we never hold s.mu across a conn
	// op. Closing the conn also unblocks a drain goroutine stuck in conn.Write
	// (a stalled tailer) so its fd/goroutine don't leak until session close.
	for _, sub := range dropped {
		_ = sub.conn.Close()
	}
	return nil
}

// subscribe registers c as a live tailer and starts its drain goroutine. The
// caller must hold no lock. If the session is already done, c is closed.
func (s *session) subscribe(c net.Conn) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		_ = c.Close()
		return
	}
	sub := &subscriber{conn: c, ch: make(chan []byte, subChanCap), done: make(chan struct{})}
	s.subs[c] = sub
	s.mu.Unlock()
	go sub.drain()
}

// drain writes queued chunks to the subscriber's conn until the channel is
// closed (lagging drop or session close) or a conn write fails.
func (sub *subscriber) drain() {
	defer close(sub.done)
	for b := range sub.ch {
		if _, err := sub.conn.Write(b); err != nil {
			// Conn is dead; keep draining so close()/write() don't block on the
			// channel, but stop writing.
			for range sub.ch {
			}
			return
		}
	}
}

func (s *session) addTailer(c net.Conn, path string) {
	// Replay what has been recorded so far (decrypted), then subscribe to live
	// plaintext bytes.
	if rc, err := store.OpenCast(path); err == nil {
		_, copyErr := io.Copy(c, rc)
		closeErr := rc.Close()
		if copyErr != nil || closeErr != nil {
			_ = c.Close()
			return
		}
	}
	s.subscribe(c)
}

func (s *session) close() error {
	s.mu.Lock()
	s.done = true
	var err error
	if s.f != nil {
		if syncErr := s.f.Sync(); syncErr != nil {
			err = syncErr
		}
		if closeErr := s.f.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		s.f = nil
	}
	subs := make([]*subscriber, 0, len(s.subs))
	for c, sub := range s.subs {
		subs = append(subs, sub)
		delete(s.subs, c)
		close(sub.ch) // stop the drain goroutine
	}
	s.mu.Unlock()
	// Close conns and wait for drain goroutines outside the lock so a slow conn
	// can't hold s.mu (which the recorder needs).
	for _, sub := range subs {
		_ = sub.conn.Close()
		<-sub.done
	}
	return err
}

type registry struct {
	mu        sync.RWMutex // RWMutex: concurrent reads (tail) don't block each other
	live      map[string]sessionRef
	key       []byte         // at-rest encryption key
	connCount map[uint32]int // per-UID connection count
	cap       int            // per-UID concurrent session cap; updatable on SIGHUP
}

type sessionRef struct {
	sess *session
	path string
}

func (r *registry) add(id string, s *session, path string) {
	r.mu.Lock()
	r.live[id] = sessionRef{s, path}
	r.mu.Unlock()
}

func (r *registry) remove(id string) {
	r.mu.Lock()
	delete(r.live, id)
	r.mu.Unlock()
}

func (r *registry) get(id string) (sessionRef, bool) {
	r.mu.RLock()
	ref, ok := r.live[id]
	r.mu.RUnlock()
	return ref, ok
}

// reserve atomically claims a session slot for uid if it is under the cap,
// returning false (and reserving nothing) when the cap is reached.
func (r *registry) reserve(uid uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.connCount[uid] >= r.cap {
		return false
	}
	r.connCount[uid]++
	return true
}

// release returns a previously reserved slot for uid.
func (r *registry) release(uid uint32) {
	r.mu.Lock()
	r.connCount[uid]--
	r.mu.Unlock()
}

// setCap updates the per-UID session cap under the lock. It is called from the
// SIGHUP handler so an edited config takes effect without a daemon restart.
func (r *registry) setCap(c int) {
	r.mu.Lock()
	r.cap = c
	r.mu.Unlock()
}

// backupConfigChanged reports whether any backup-relevant field differs.
// Pure helper — no I/O — so it can be unit-tested without goroutines.
func backupConfigChanged(prev, nc *config.Config) bool {
	return prev.BackupType != nc.BackupType ||
		prev.BackupTarget != nc.BackupTarget ||
		prev.BackupIntervalSec != nc.BackupIntervalSec
}

// Run starts the daemon: ingests stray user-local recordings, then serves the
// unix socket until ctx is cancelled or the process is terminated. cfg supplies
// the initial socket path and per-UID session cap. reload, if non-nil, delivers
// freshly-parsed configs (e.g. from a SIGHUP handler); each one re-applies the
// safely-reloadable fields (log level, session cap) without a restart.
func Run(ctx context.Context, cfg *config.Config, reload <-chan *config.Config) error {
	socketPath := cfg.SocketPath
	if err := os.MkdirAll(store.CentralDir(), 0o700); err != nil {
		return fmt.Errorf("create central dir: %w", err)
	}
	// Enforce root-only perms even if the dir pre-existed with looser modes.
	if err := os.Chmod(store.CentralDir(), 0o700); err != nil {
		return fmt.Errorf("chmod central dir: %w", err)
	}

	key, err := ensureKey()
	if err != nil {
		return err
	}

	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", socketPath, err)
	}
	// Any user may connect to be recorded; access control on the *files* is
	// what enforces root-only reads.
	if err := os.Chmod(socketPath, 0o666); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}

	// Start ingest after the socket is ready so connections aren't dropped
	// while a large spool is being processed.
	go ingestLocalRecordings(key)
	backupCfgCh := make(chan *config.Config, 1)
	go backupLoop(ctx, cfg, backupCfgCh, backup.Run)

	// Close the listener when the context is cancelled so Accept() unblocks.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	reg := &registry{live: map[string]sessionRef{}, key: key, connCount: map[uint32]int{}, cap: cfg.SessionCap}
	logger.Infof("ttrackd: listening on %s, storing in %s (encrypted)", socketPath, store.CentralDir())

	// Apply hot-reloadable config on SIGHUP-delivered configs. socket_path and
	// central_dir cannot change at runtime — they require a restart.
	if reload != nil {
		go func() {
			prevBackupType := cfg.BackupType
			prevBackupTarget := cfg.BackupTarget
			prevBackupIntervalSec := cfg.BackupIntervalSec
			for {
				select {
				case <-ctx.Done():
					return
				case nc, ok := <-reload:
					if !ok {
						return
					}
					logger.Set(logger.Level(nc.LogLevel))
					reg.setCap(nc.SessionCap)
					logger.Infof("ttrackd: config reloaded (SIGHUP): log_level=%d session_cap=%d", nc.LogLevel, nc.SessionCap)
					if nc.SocketPath != socketPath || nc.CentralDir != store.CentralDir() {
						logger.Infof("ttrackd: socket_path / central_dir changes require a restart to take effect")
					}
					if backupConfigChanged(
						&config.Config{BackupType: prevBackupType, BackupTarget: prevBackupTarget, BackupIntervalSec: prevBackupIntervalSec},
						nc,
					) {
						select {
						case backupCfgCh <- nc:
						default:
							// A reload is already queued; the latest config will be received on next tick.
						}
						prevBackupType = nc.BackupType
						prevBackupTarget = nc.BackupTarget
						prevBackupIntervalSec = nc.BackupIntervalSec
						logger.Infof("ttrackd: backup config reloaded (type=%s target=%s interval=%ds)",
							nc.BackupType, nc.BackupTarget, nc.BackupIntervalSec)
					}
				}
			}
		}()
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				logger.Infof("ttrackd: shutting down")
				return nil
			}
			// Transient errors (e.g. EINTR): log and retry.
			// Permanent errors (listener closed, fd exhausted): stop.
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				logger.Debugf("ttrackd: accept timeout: %v", err)
				continue
			}
			// Check if it's a temporary error
			if isTemporary(err) {
				logger.Warnf("ttrackd: accept (retrying): %v", err)
				continue
			}
			return fmt.Errorf("ttrackd: accept fatal: %w", err)
		}
		uc, ok := conn.(*net.UnixConn)
		if !ok {
			// Should never happen with a unix listener, but guard the cast.
			logger.Errorf("ttrackd: unexpected conn type %T", conn)
			_ = conn.Close()
			continue
		}
		go handle(uc, reg)
	}
}

func handle(conn *net.UnixConn, reg *registry) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("ttrackd: panic in handle: %v", r)
		}
	}()

	cred, err := peerCred(conn)
	if err != nil || cred == nil {
		return
	}

	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	line = strings.TrimSpace(line)

	switch {
	case line == "REC":
		handleRec(conn, br, cred, reg)
	case strings.HasPrefix(line, "TAIL "):
		handleTail(conn, strings.TrimSpace(line[5:]), cred, reg)
	case strings.HasPrefix(line, "ANSIBLE "):
		handleAnsible(conn, br, strings.TrimSpace(line[8:]), cred, reg)
	default:
		_, _ = conn.Write([]byte("ERR unknown command\n"))
	}
}

func handleRec(conn *net.UnixConn, br *bufio.Reader, cred *unix.Ucred, reg *registry) {
	if !reg.reserve(cred.Uid) {
		_, _ = conn.Write([]byte("ERR too many sessions\n"))
		return
	}
	defer reg.release(cred.Uid)

	uname := lookupUser(cred.Uid)
	dir := filepath.Join(store.CentralDir(), uname)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = os.Chmod(dir, 0o700)

	id := fmt.Sprintf("%s-%d", time.Now().Format("20060102T150405.000000000"), cred.Pid)
	path := filepath.Join(dir, id+".cast")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		logger.Errorf("ttrackd: open session file %s: %v", path, err)
		_, _ = conn.Write([]byte("ERR session file unavailable\n"))
		return
	}
	enc, err := crypto.NewWriter(f, reg.key)
	if err != nil {
		f.Close()
		return
	}

	sess := &session{f: f, enc: enc, subs: map[net.Conn]*subscriber{}}
	reg.add(id, sess, path)
	logger.Infof("ttrackd: session started  user=%-20s id=%s", uname, id)
	var totalBytes int64
	defer func() {
		if err := sess.close(); err != nil {
			logger.Warnf("ttrackd: close %s: %v", path, err)
		}
		reg.remove(id)
		logger.Infof("ttrackd: session closed   user=%-20s id=%s bytes=%d", uname, id, totalBytes)
	}()

	buf := make([]byte, 32*1024)
	for {
		n, rerr := br.Read(buf)
		if n > 0 {
			totalBytes += int64(n)
			if err := sess.write(buf[:n]); err != nil {
				logger.Warnf("ttrackd: write %s (uid=%d): %v — session truncated", path, cred.Uid, err)
				return
			}
		}
		if rerr != nil {
			return
		}
	}
}

func handleTail(conn *net.UnixConn, id string, cred *unix.Ucred, reg *registry) {
	if cred.Uid != 0 {
		_, _ = conn.Write([]byte("ERR tail requires root\n"))
		return
	}
	id = strings.TrimSuffix(id, ".cast")
	ref, ok := reg.get(id)
	if !ok {
		_, _ = conn.Write([]byte("ERR no active session " + id + "\n"))
		return
	}
	ref.sess.addTailer(conn, ref.path)
	// Block until the recorder ends (session.close() closes our conn) or the
	// tailer disconnects.
	_, _ = io.Copy(io.Discard, conn)
}

// handleAnsible stores an Ansible playbook run from `ttrack ansible-ingest`.
// The run id is already validated by the ingest process but we re-validate
// here before using it as a path component.
func handleAnsible(conn *net.UnixConn, br *bufio.Reader, runID string, cred *unix.Ucred, reg *registry) {
	if !ansible.ValidRunID(runID) {
		_, _ = conn.Write([]byte("ERR invalid ansible run id\n"))
		return
	}

	uname := lookupUser(cred.Uid)
	dir := filepath.Join(store.CentralDir(), uname, "ansible")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = os.Chmod(dir, 0o700)

	path := filepath.Join(dir, runID+".ajsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		logger.Errorf("ttrackd: open ansible file %s: %v", path, err)
		_, _ = conn.Write([]byte("ERR ansible file unavailable\n"))
		return
	}
	enc, err := crypto.NewWriter(f, reg.key)
	if err != nil {
		f.Close()
		return
	}
	logger.Infof("ttrackd: ansible run started user=%-20s run=%s", uname, runID)
	var ansibleBytes int64
	defer func() {
		logger.Infof("ttrackd: ansible run stored  user=%-20s run=%s bytes=%d", uname, runID, ansibleBytes)
	}()
	defer f.Close()
	defer f.Sync()

	buf := make([]byte, 32*1024)
	for {
		n, rerr := br.Read(buf)
		if n > 0 {
			ansibleBytes += int64(n)
			if _, werr := enc.Write(buf[:n]); werr != nil {
				return
			}
		}
		if rerr != nil {
			return
		}
	}
}

func peerCred(c *net.UnixConn) (*unix.Ucred, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *unix.Ucred
	var cerr error
	if err := raw.Control(func(fd uintptr) {
		cred, cerr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	return cred, cerr
}

func lookupUser(uid uint32) string {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil || u.Username == "" {
		return strconv.FormatUint(uint64(uid), 10)
	}
	// Validate username is safe to use as a path component.
	uname := u.Username
	for _, c := range uname {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return strconv.FormatUint(uint64(uid), 10)
		}
	}
	if uname == "." || uname == ".." || strings.HasPrefix(uname, ".") || strings.Contains(uname, "/") {
		return strconv.FormatUint(uint64(uid), 10)
	}
	return uname
}

// ensureKey loads the at-rest key, creating it on first run. It refuses to
// create a new key when recordings already exist (a new key would make them
// permanently unreadable) — the operator must restore the original key.
func ensureKey() ([]byte, error) {
	kp := store.KeyPath()
	data, err := os.ReadFile(kp)
	if err == nil {
		if len(data) != crypto.KeySize {
			return nil, fmt.Errorf("key %s has wrong size %d", kp, len(data))
		}
		setImmutable(kp) // best-effort; idempotent
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key %s: %w", kp, err)
	}
	if encryptedRecordingsExist() {
		return nil, fmt.Errorf("encryption key %s is missing but ENCRYPTED recordings exist in %s; "+
			"restore the original key — refusing to start (a new key would make them permanently unreadable)",
			kp, store.CentralDir())
	}
	key, gerr := crypto.GenerateKey()
	if gerr != nil {
		return nil, gerr
	}
	if werr := os.WriteFile(kp, key, 0o600); werr != nil {
		return nil, fmt.Errorf("write key %s: %w", kp, werr)
	}
	_ = os.Chmod(kp, 0o600)
	setImmutable(kp) // protect from rm/vi/sed/>/tee even by root (until chattr -i)
	logger.Infof("ttrackd: created NEW encryption key at %s — BACK IT UP NOW. "+
		"Losing it makes every recording permanently unreadable.", kp)
	return key, nil
}

// setImmutable sets the FS immutable flag (chattr +i) on path so it cannot be
// modified, deleted, or renamed — even by root — until `chattr -i`. Best-effort:
// silently skips on filesystems that do not support it.
func setImmutable(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	flags, err := unix.IoctlGetInt(int(f.Fd()), unix.FS_IOC_GETFLAGS)
	if err != nil {
		return // e.g. EOPNOTSUPP / ENOTTY on unsupported fs
	}
	const fsImmutableFL = 0x00000010 // FS_IMMUTABLE_FL (stable kernel ABI)
	if flags&fsImmutableFL != 0 {
		return // already immutable
	}
	_ = unix.IoctlSetPointerInt(int(f.Fd()), unix.FS_IOC_SETFLAGS, flags|fsImmutableFL)
}

// encryptedRecordingsExist reports whether any central cast is already
// encrypted (magic-prefixed). Plaintext recordings from before encryption was
// enabled do not need a key and are not a reason to refuse startup.
func encryptedRecordingsExist() bool {
	users, err := store.Users()
	if err != nil {
		return false
	}
	for _, u := range users {
		names, _ := store.UserSessions(u)
		for _, n := range names {
			f, err := os.Open(filepath.Join(store.CentralDir(), u, n))
			if err != nil {
				continue
			}
			magic := make([]byte, len(crypto.Magic))
			if _, err := io.ReadFull(f, magic); err != nil {
				f.Close()
				continue // unreadable file — skip
			}
			f.Close()
			if string(magic) == crypto.Magic {
				return true
			}
		}
	}
	return false
}

// ingestLocalRecordings sweeps per-user local recordings into the central
// store on startup, encrypting them so sessions recorded while the daemon was
// down become root-only. Source files are removed after a successful copy.
func ingestLocalRecordings(key []byte) {
	for _, home := range homeDirs() {
		ingestHome(home, key)
	}
}

// homeDirs returns the set of home directories to sweep for stray local
// recordings. It enumerates real accounts from /etc/passwd (covering LDAP/SSSD
// homes under /var/home, service accounts, etc.) and always includes /root.
// If /etc/passwd can't be read it falls back to the old /root + /home/* scan.
func homeDirs() []string {
	if content, err := os.ReadFile("/etc/passwd"); err == nil {
		return homeDirsFromPasswd(content)
	}
	homes := []string{"/root"}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				homes = append(homes, filepath.Join("/home", e.Name()))
			}
		}
	}
	return homes
}

// homeDirsFromPasswd extracts the absolute home directories (field 6) from
// /etc/passwd content. /root is always included; results are deduped and
// malformed/relative entries are skipped. It is pure so it can be unit-tested.
func homeDirsFromPasswd(content []byte) []string {
	seen := map[string]struct{}{"/root": {}}
	homes := []string{"/root"}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		home := fields[5]
		if !strings.HasPrefix(home, "/") {
			continue
		}
		if _, ok := seen[home]; ok {
			continue
		}
		seen[home] = struct{}{}
		homes = append(homes, home)
	}
	return homes
}

func ingestHome(home string, key []byte) {
	uname := filepath.Base(home)
	src := filepath.Join(home, ".local", "share", "ttrack")
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	dstDir := filepath.Join(store.CentralDir(), uname)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".cast" || store.IsActive(e.Name()) {
			continue
		}
		if err := os.MkdirAll(dstDir, 0o700); err != nil {
			continue
		}
		sp := filepath.Join(src, e.Name())
		if copyFile(sp, filepath.Join(dstDir, e.Name()), key) == nil {
			_ = os.Remove(sp)
		}
	}
}

// copyFileAfterCopy is a test hook fired after the bytes are copied but before
// the post-copy re-check, used to simulate the source changing mid-copy. nil in
// production.
var copyFileAfterCopy func()

// backupTickUnit is multiplied by BackupIntervalSec to produce the ticker
// interval. Tests override this to time.Millisecond for fast ticks.
var backupTickUnit = time.Second

// backupLoop runs the periodic backup goroutine. A nil tickC (when backup is
// disabled) is never selected, so the loop parks cheaply on ctx.Done() and
// updates only. Reload via the updates channel resets the ticker.
func backupLoop(
	ctx context.Context,
	initial *config.Config,
	updates <-chan *config.Config,
	runFn func(*config.Config) error,
) {
	cur := initial
	var ticker *time.Ticker
	var tickC <-chan time.Time

	resetTicker := func(cfg *config.Config) {
		if ticker != nil {
			ticker.Stop()
			ticker = nil
			tickC = nil
		}
		if cfg.BackupIntervalSec > 0 && cfg.BackupType != "" {
			ticker = time.NewTicker(time.Duration(cfg.BackupIntervalSec) * backupTickUnit)
			tickC = ticker.C
		}
	}
	resetTicker(cur)
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case nc, ok := <-updates:
			if !ok {
				return
			}
			cur = nc
			resetTicker(cur)
		case <-tickC:
			if err := runFn(cur); err != nil {
				logger.Errorf("ttrackd: backup failed: %v", err)
			} else {
				logger.Infof("ttrackd: backup completed (type=%s target=%s)", cur.BackupType, cur.BackupTarget)
			}
		}
	}
}

// copyFile copies a user-owned plaintext source into the root-only central
// store, encrypting it. The source is opened O_NOFOLLOW and verified to be a
// regular file, so a user cannot symlink a recording at a root-readable target
// (e.g. /etc/shadow) and have the root daemon copy it into the central store.
//
// To avoid capturing a truncated in-progress recording, copyFile snapshots the
// source size up front and re-checks after copying: if the recording became
// active again or its size changed, the partial destination is removed and an
// error is returned so the caller leaves the source for a later run.
func copyFile(src, dst string, key []byte) (retErr error) {
	in, err := os.OpenFile(src, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err // ELOOP if src is a symlink
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("ingest: %s is not a regular file", src)
	}
	startSize := fi.Size()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// On any error, the partial destination must not be left behind.
	defer func() {
		closeErr := out.Close()
		if retErr == nil && closeErr != nil {
			retErr = closeErr
		}
		if retErr != nil {
			_ = os.Remove(dst)
		}
	}()
	enc, err := crypto.NewWriter(out, key)
	if err != nil {
		return err
	}
	if _, err := io.Copy(enc, in); err != nil {
		return err
	}
	if copyFileAfterCopy != nil {
		copyFileAfterCopy()
	}
	// Re-check: a recording that became active again, or whose size changed
	// during the copy, may have been captured truncated — skip it this run.
	if store.IsActive(filepath.Base(src)) {
		return fmt.Errorf("ingest: %s became active during copy — skipping", src)
	}
	cur, err := in.Stat()
	if err != nil {
		return err
	}
	if cur.Size() != startSize {
		return fmt.Errorf("ingest: %s size changed during copy (%d -> %d) — skipping",
			src, startSize, cur.Size())
	}
	return out.Sync()
}
