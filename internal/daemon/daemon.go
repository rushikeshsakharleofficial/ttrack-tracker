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

	"ttrack/internal/crypto"
	"ttrack/internal/store"
)

type session struct {
	mu   sync.Mutex
	f    *os.File  // underlying file, for sync/close
	enc  io.Writer // encrypting writer over f; recorded bytes land as ciphertext
	subs map[net.Conn]struct{}
	done bool
}

func (s *session) write(b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enc != nil {
		if _, err := s.enc.Write(b); err != nil { // encrypted to disk
			return err
		}
	}
	for c := range s.subs {
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Write(b); err != nil {
			delete(s.subs, c)
			_ = c.Close()
		}
	}
	return nil
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		_ = c.Close()
		return
	}
	s.subs[c] = struct{}{}
}

func (s *session) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	for c := range s.subs {
		_ = c.Close()
		delete(s.subs, c)
	}
	return err
}

type registry struct {
	mu   sync.Mutex
	live map[string]sessionRef
	key  []byte // at-rest encryption key
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
	r.mu.Lock()
	ref, ok := r.live[id]
	r.mu.Unlock()
	return ref, ok
}

// Run starts the daemon: ingests stray user-local recordings, then serves the
// unix socket until the process is terminated.
func Run(socketPath string) error {
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
	ingestLocalRecordings(key)

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

	reg := &registry{live: map[string]sessionRef{}, key: key}
	fmt.Fprintf(os.Stderr, "ttrackd: listening on %s, storing in %s (encrypted)\n",
		socketPath, store.CentralDir())

	for {
		conn, err := ln.Accept()
		if err != nil {
			// A single bad accept must not kill the recorder service.
			fmt.Fprintf(os.Stderr, "ttrackd: accept: %v\n", err)
			continue
		}
		go handle(conn.(*net.UnixConn), reg)
	}
}

func handle(conn *net.UnixConn, reg *registry) {
	defer conn.Close()

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
		return
	}
	enc, err := crypto.NewWriter(f, reg.key)
	if err != nil {
		f.Close()
		return
	}

	sess := &session{f: f, enc: enc, subs: map[net.Conn]struct{}{}}
	reg.add(id, sess, path)
	defer func() {
		if err := sess.close(); err != nil {
			fmt.Fprintf(os.Stderr, "ttrackd: close %s: %v\n", path, err)
		}
		reg.remove(id)
	}()

	buf := make([]byte, 32*1024)
	for {
		n, rerr := br.Read(buf)
		if n > 0 {
			if err := sess.write(buf[:n]); err != nil {
				fmt.Fprintf(os.Stderr, "ttrackd: write %s: %v\n", path, err)
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
	if !validAnsibleRunID(runID) {
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
		return
	}
	enc, err := crypto.NewWriter(f, reg.key)
	if err != nil {
		f.Close()
		return
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	for {
		n, rerr := br.Read(buf)
		if n > 0 {
			if _, werr := enc.Write(buf[:n]); werr != nil {
				return
			}
		}
		if rerr != nil {
			return
		}
	}
}

// validAnsibleRunID accepts only safe characters (alphanumeric, -, _, T) so
// the run id can be used directly as a filename without path traversal risk.
func validAnsibleRunID(id string) bool {
	if len(id) < 5 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == 'T') {
			return false
		}
	}
	return true
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
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil && u.Username != "" {
		return u.Username
	}
	return strconv.FormatUint(uint64(uid), 10)
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
	fmt.Fprintf(os.Stderr,
		"ttrackd: created NEW encryption key at %s — BACK IT UP NOW. "+
			"Losing it makes every recording permanently unreadable.\n", kp)
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
			rn, _ := io.ReadFull(f, magic)
			f.Close()
			if rn == len(crypto.Magic) && string(magic) == crypto.Magic {
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

func homeDirs() []string {
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

// copyFile copies a user-owned plaintext source into the root-only central
// store, encrypting it. The source is opened O_NOFOLLOW and verified to be a
// regular file, so a user cannot symlink a recording at a root-readable target
// (e.g. /etc/shadow) and have the root daemon copy it into the central store.
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
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); retErr == nil && err != nil {
			retErr = err
		}
	}()
	enc, err := crypto.NewWriter(out, key)
	if err != nil {
		return err
	}
	if _, err := io.Copy(enc, in); err != nil {
		return err
	}
	return out.Sync()
}
