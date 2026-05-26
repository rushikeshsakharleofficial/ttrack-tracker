package crypto

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRoundTripMultiChunk(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	var buf bytes.Buffer
	w, err := NewWriter(&buf, key)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	chunks := []string{
		`{"version":2,"width":80,"height":24}` + "\n",
		`[0.1, "o", "hello\r\n"]` + "\n",
		`[0.2, "o", "world\r\n"]` + "\n",
	}
	for _, c := range chunks {
		if _, err := w.Write([]byte(c)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	// On-disk bytes must be opaque: the plaintext must not appear.
	if bytes.Contains(buf.Bytes(), []byte("hello")) {
		t.Fatal("plaintext 'hello' found in ciphertext output")
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte(Magic)) {
		t.Fatal("missing magic prefix")
	}

	// Read back: consume magic, then decrypt.
	r := bytes.NewReader(buf.Bytes())
	magic := make([]byte, len(Magic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != Magic {
		t.Fatalf("magic read: %v %q", err, magic)
	}
	dr, err := NewReader(r, key)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	got, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	want := strings.Join(chunks, "")
	if string(got) != want {
		t.Fatalf("round-trip mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestTruncatedTrailingFrameIsEOF(t *testing.T) {
	key, _ := GenerateKey()
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, key)
	_, _ = w.Write([]byte("first frame intact\n"))
	_, _ = w.Write([]byte("second frame will be truncated\n"))

	full := buf.Bytes()
	// Chop the last 5 bytes to simulate a daemon dying mid-write.
	truncated := full[:len(full)-5]

	r := bytes.NewReader(truncated)
	magic := make([]byte, len(Magic))
	_, _ = io.ReadFull(r, magic)
	dr, _ := NewReader(r, key)
	got, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("expected nil error (truncation -> EOF), got %v", err)
	}
	if !strings.Contains(string(got), "first frame intact") {
		t.Fatalf("first complete frame should be recovered, got %q", got)
	}
	if strings.Contains(string(got), "truncated") {
		t.Fatalf("partial trailing frame should be dropped, got %q", got)
	}
}

func TestWrongKeyFails(t *testing.T) {
	k1, _ := GenerateKey()
	k2, _ := GenerateKey()
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, k1)
	_, _ = w.Write([]byte("secret session data\n"))

	r := bytes.NewReader(buf.Bytes())
	magic := make([]byte, len(Magic))
	_, _ = io.ReadFull(r, magic)
	dr, _ := NewReader(r, k2)
	got, _ := io.ReadAll(dr)
	if strings.Contains(string(got), "secret") {
		t.Fatal("wrong key must not decrypt plaintext")
	}
}
