// Package daemon implements ttrackd: the root collector that receives session
// recordings from per-user `ttrack rec` clients, stores them in the central
// root-only store, and fans out live sessions to root tailers.
//
// Protocol (line-based handshake, then raw asciinema v2 cast bytes):
//
//	client -> "REC\n"            then streams cast bytes (recorder)
//	client -> "TAIL <id>\n"      then reads live cast bytes (root only)
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

	"ttrack/internal/store"
)

type session struct {
	mu   sync.Mutex
	f    *os.File
	subs map[net.Conn]struct{}
	done bool
}

func (s *session) write(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		_, _ = s.f.Write(b)
	}
	for c := range s.subs {
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Write(b); err != nil {
			delete(s.subs, c)
			_ = c.Close()
		}
	}
}

func (s *session) addTailer(c net.Conn, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Replay what has been recorded so far, then subscribe to live bytes.
	if data, err := os.ReadFile(path); err == nil {
		_, _ = c.Write(data)
	}
	s.subs[c] = struct{}{}
}

func (s *session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	if s.f != nil {
		_ = s.f.Sync()
		_ = s.f.Close()
		s.f = nil
	}
	for c := range s.subs {
		_ = c.Close()
		delete(s.subs, c)
	}
}

type registry struct {
	mu   sync.Mutex
	live map[string]sessionRef
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
	ingestLocalRecordings()

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

	reg := &registry{live: map[string]sessionRef{}}
	fmt.Fprintf(os.Stderr, "ttrackd: listening on %s, storing in %s\n",
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

	id := fmt.Sprintf("%s-%d", time.Now().Format("20060102T150405"), cred.Pid)
	path := filepath.Join(dir, id+".cast")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}

	sess := &session{f: f, subs: map[net.Conn]struct{}{}}
	reg.add(id, sess, path)
	defer func() {
		sess.close()
		reg.remove(id)
	}()

	buf := make([]byte, 32*1024)
	for {
		n, rerr := br.Read(buf)
		if n > 0 {
			sess.write(buf[:n])
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

// ingestLocalRecordings sweeps per-user local recordings into the central
// store on startup, so sessions recorded while the daemon was down become
// root-only. Source files are removed after a successful copy.
func ingestLocalRecordings() {
	homes := []string{"/root"}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				homes = append(homes, filepath.Join("/home", e.Name()))
			}
		}
	}
	for _, home := range homes {
		uname := filepath.Base(home)
		src := filepath.Join(home, ".local", "share", "ttrack")
		entries, err := os.ReadDir(src)
		if err != nil {
			continue
		}
		dstDir := filepath.Join(store.CentralDir(), uname)
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".cast" {
				continue
			}
			if store.IsActive(e.Name()) {
				continue // still being written; leave it
			}
			if err := os.MkdirAll(dstDir, 0o700); err != nil {
				continue
			}
			sp := filepath.Join(src, e.Name())
			dp := filepath.Join(dstDir, e.Name())
			if copyFile(sp, dp) == nil {
				_ = os.Remove(sp)
			}
		}
	}
}

// copyFile copies a user-owned source into the root-only central store.
// The source is opened O_NOFOLLOW and verified to be a regular file, so a
// user cannot symlink a recording at a root-readable target (e.g.
// /etc/shadow) and have the root daemon copy it into the central store.
func copyFile(src, dst string) error {
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
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
