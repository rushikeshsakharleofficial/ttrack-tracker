package daemon

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestSession builds a session whose disk writes go to an in-memory buffer
// (enc) so tests can exercise fan-out without the store/crypto stack.
func newTestSession() (*session, *bytes.Buffer) {
	var disk bytes.Buffer
	s := &session{enc: &disk, subs: map[net.Conn]*subscriber{}}
	return s, &disk
}

// --- #1: tailer fan-out decoupling ----------------------------------------

// A stalled subscriber (its conn is never read) must not block disk writes nor
// a fast subscriber. The slow sub eventually gets dropped; writes keep flowing.
func TestSlowSubscriberDoesNotBlockWrites(t *testing.T) {
	s, disk := newTestSession()

	// Fast subscriber: drained continuously.
	fastSrv, fastCli := net.Pipe()
	defer fastSrv.Close()
	defer fastCli.Close()
	s.subscribe(fastSrv)

	gotFast := make(chan int, 1)
	go func() {
		n := 0
		b := make([]byte, 4096)
		for {
			r, err := fastCli.Read(b)
			n += r
			if err != nil {
				gotFast <- n
				return
			}
		}
	}()

	// Slow/stalled subscriber: server side registered, client side NEVER read.
	slowSrv, slowCli := net.Pipe()
	defer slowSrv.Close()
	defer slowCli.Close()
	s.subscribe(slowSrv)

	// Write far more chunks than the subscriber channel can buffer. If writes
	// blocked on the stalled subscriber this would deadlock; with a deadline we
	// assert it returns promptly.
	payload := bytes.Repeat([]byte("x"), 1024)
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 5000; i++ {
			if err := s.write(payload); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writes blocked on a stalled subscriber (head-of-line blocking)")
	}

	// All bytes must have reached disk synchronously and in order.
	if disk.Len() != 5000*len(payload) {
		t.Fatalf("disk got %d bytes, want %d", disk.Len(), 5000*len(payload))
	}

	// Closing the session releases everything (fast reader sees EOF).
	if err := s.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-gotFast:
	case <-time.After(5 * time.Second):
		t.Fatal("fast subscriber did not unblock after session close")
	}
}

// A subscriber receives the bytes written after it subscribed, even when the
// caller reuses its buffer between writes (the session must copy).
func TestSubscriberGetsCopiedBytes(t *testing.T) {
	s, _ := newTestSession()

	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	s.subscribe(srv)

	got := make(chan []byte, 1)
	go func() {
		b := make([]byte, 4)
		_, _ = io.ReadFull(cli, b)
		got <- append([]byte(nil), b...)
	}()

	// Reused buffer: write "AAAA", then immediately overwrite it.
	buf := []byte("AAAA")
	if err := s.write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	copy(buf, "BBBB") // mutate the caller buffer after handing it off

	select {
	case b := <-got:
		if string(b) != "AAAA" {
			t.Fatalf("subscriber got %q, want %q — bytes not copied", b, "AAAA")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscriber received nothing")
	}
	_ = s.close()
}

// Closing the session must release a live (never-dropped) subscriber: its conn
// is closed so a blocked Read unblocks with an error. This exercises close()'s
// release path specifically (the subscriber stays well under the channel cap so
// write() never drops it).
func TestCloseReleasesLiveSubscriber(t *testing.T) {
	s, _ := newTestSession()

	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	s.subscribe(srv)

	// A reader that consumes everything, then reports the error it eventually
	// sees (EOF/closed pipe once the session releases the conn).
	readErr := make(chan error, 1)
	go func() {
		b := make([]byte, 64)
		for {
			if _, err := cli.Read(b); err != nil {
				readErr <- err
				return
			}
		}
	}()

	// A few small writes, far below subChanCap, so the subscriber is never the
	// lagging one and remains registered until close().
	for i := 0; i < 4; i++ {
		if err := s.write([]byte("hello")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	if err := s.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-readErr:
	case <-time.After(5 * time.Second):
		t.Fatal("close() did not release the live subscriber's conn")
	}
}

// --- #2: homeDirsFromPasswd parser ----------------------------------------

func TestHomeDirsFromPasswd(t *testing.T) {
	content := []byte(strings.Join([]string{
		"root:x:0:0:root:/root:/bin/bash",
		"daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin",
		"alice:x:1000:1000:Alice:/home/alice:/bin/bash",
		"bob:x:1001:1001:Bob:/var/home/bob:/bin/zsh",
		"# a comment line",
		"malformed-line-without-fields",
		"dup:x:1002:1002::/home/alice:/bin/bash", // duplicate home -> deduped
		"",
	}, "\n"))

	got := homeDirsFromPasswd(content)
	sort.Strings(got)

	want := []string{"/home/alice", "/root", "/usr/sbin", "/var/home/bob"}
	if len(got) != len(want) {
		t.Fatalf("homeDirsFromPasswd = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("homeDirsFromPasswd = %v, want %v", got, want)
		}
	}
}

// --- #4: per-UID session cap is configurable -------------------------------

// reserve must honor the registry's cap, and setCap must change it under the
// lock so SIGHUP reloads take effect without a restart.
func TestRegistryCap(t *testing.T) {
	reg := &registry{live: map[string]sessionRef{}, connCount: map[uint32]int{}, cap: 2}

	const uid = uint32(1000)
	if !reg.reserve(uid) || !reg.reserve(uid) {
		t.Fatal("first two reservations under cap=2 should succeed")
	}
	if reg.reserve(uid) {
		t.Fatal("third reservation should be rejected at cap=2")
	}

	// Releasing one slot lets a new reservation through.
	reg.release(uid)
	if !reg.reserve(uid) {
		t.Fatal("reservation after release should succeed")
	}

	// Raising the cap (as SIGHUP would) admits more sessions immediately.
	reg.setCap(5)
	if !reg.reserve(uid) || !reg.reserve(uid) || !reg.reserve(uid) {
		t.Fatal("reservations up to the raised cap=5 should succeed")
	}
	if reg.reserve(uid) {
		t.Fatal("reservation beyond raised cap=5 should be rejected")
	}
}

// setCap, reserve and release must be race-free (run under -race).
func TestRegistryCapRace(t *testing.T) {
	reg := &registry{live: map[string]sessionRef{}, connCount: map[uint32]int{}, cap: 10}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(uid uint32) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if reg.reserve(uid) {
					reg.release(uid)
				}
			}
		}(uint32(i))
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			reg.setCap(j%20 + 1)
		}
	}()
	wg.Wait()
}

// --- #3: in-progress local recordings are not ingested truncated -----------

// copyFile succeeds for a stable source: the destination is a valid encrypted
// copy and the source is left intact (removal is the caller's job).
func TestCopyFileStable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "20200101T000000.000000000-1.cast")
	if err := os.WriteFile(src, bytes.Repeat([]byte("data"), 100), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out.cast")
	key := bytes.Repeat([]byte("k"), 32)

	if err := copyFile(src, dst, key); err != nil {
		t.Fatalf("copyFile on stable source: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("destination missing after successful copy: %v", err)
	}
}

// If the source grows during the copy, copyFile must detect the size change,
// remove the partial destination, and return an error so the caller leaves the
// source in place for a later run.
func TestCopyFileSizeChangedRemovesPartial(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "20200101T000000.000000000-1.cast")
	if err := os.WriteFile(src, bytes.Repeat([]byte("data"), 100), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out.cast")
	key := bytes.Repeat([]byte("k"), 32)

	// Hook fires after the snapshot+copy, before the re-check: append to the
	// source so its size differs from the snapshot.
	copyFileAfterCopy = func() {
		f, err := os.OpenFile(src, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		_, _ = f.Write([]byte("more"))
		_ = f.Close()
	}
	defer func() { copyFileAfterCopy = nil }()

	if err := copyFile(src, dst, key); err == nil {
		t.Fatal("copyFile should fail when the source changes during copy")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("partial destination should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should be left in place, stat err=%v", err)
	}
}
