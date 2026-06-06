// Package crypto provides at-rest encryption for cast files: a framed
// AES-256-GCM stream so a recording on disk is opaque to cat/strings/grep and
// is readable only with the key.
//
// File layout: a magic prefix, then a sequence of frames:
//
//	"TTEC1\n"
//	[4-byte big-endian frame length][12-byte nonce][ciphertext + 16-byte tag]
//	...
//
// Each Write to the writer becomes one frame with a fresh random nonce. The
// reader decrypts frames in order; a truncated trailing frame is treated as
// end-of-stream (a recording whose daemon died mid-write is still readable up
// to the last complete frame). Corrupt or tampered complete frames return an
// explicit error instead of looking like a normal EOF.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Magic marks an encrypted cast file.
const Magic = "TTEC1\n"

// KeySize is the AES-256 key length in bytes.
const KeySize = 32

// ErrCorrupt is returned when an encrypted stream contains an invalid frame or
// a frame that fails AES-GCM authentication. A truncated trailing frame still
// returns io.EOF so sessions interrupted mid-write remain readable up to the
// last complete frame.
var ErrCorrupt = errors.New("crypto: corrupt encrypted stream")

// GenerateKey returns a new random 32-byte key.
func GenerateKey() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

type encWriter struct {
	w   io.Writer
	gcm cipher.AEAD
}

// NewWriter writes the magic prefix and returns a writer that frames each Write
// as an encrypted block.
func NewWriter(w io.Writer, key []byte) (io.Writer, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, Magic); err != nil {
		return nil, err
	}
	return &encWriter{w: w, gcm: gcm}, nil
}

func (e *encWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return 0, err
	}
	ct := e.gcm.Seal(nil, nonce, p, nil)
	var lenbuf [4]byte
	binary.BigEndian.PutUint32(lenbuf[:], uint32(len(nonce)+len(ct)))
	if _, err := e.w.Write(lenbuf[:]); err != nil {
		return 0, err
	}
	if _, err := e.w.Write(nonce); err != nil {
		return 0, err
	}
	if _, err := e.w.Write(ct); err != nil {
		return 0, err
	}
	return len(p), nil
}

type decReader struct {
	r   io.Reader
	gcm cipher.AEAD
	buf []byte
}

// NewReader returns a reader yielding the decrypted plaintext stream. The
// caller must have already consumed the magic prefix.
func NewReader(r io.Reader, key []byte) (io.Reader, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return &decReader{r: r, gcm: gcm}, nil
}

func (d *decReader) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		var lenbuf [4]byte
		if _, err := io.ReadFull(d.r, lenbuf[:]); err != nil {
			// Clean end, or a daemon dying while starting the next frame: stop at
			// the last complete frame already returned.
			return 0, io.EOF
		}
		flen := binary.BigEndian.Uint32(lenbuf[:])
		if int(flen) < d.gcm.NonceSize() {
			return 0, fmt.Errorf("%w: frame length %d smaller than nonce size %d", ErrCorrupt, flen, d.gcm.NonceSize())
		}
		const maxFrameSize = 1 << 20 // 1 MiB — largest expected PTY write chunk
		if flen > maxFrameSize {
			return 0, fmt.Errorf("%w: frame length %d exceeds maximum %d", ErrCorrupt, flen, maxFrameSize)
		}
		frame := make([]byte, flen)
		if _, err := io.ReadFull(d.r, frame); err != nil {
			// Preserve the fail-open recovery property for a daemon interrupted
			// mid-write: return bytes up to the last complete frame.
			return 0, io.EOF
		}
		ns := d.gcm.NonceSize()
		pt, err := d.gcm.Open(nil, frame[:ns], frame[ns:], nil)
		if err != nil {
			return 0, fmt.Errorf("%w: frame authentication failed", ErrCorrupt)
		}
		d.buf = pt
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}
