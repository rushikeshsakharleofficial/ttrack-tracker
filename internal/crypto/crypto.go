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
// reader decrypts frames in order; a truncated or corrupt trailing frame is
// treated as end-of-stream (a recording whose daemon died mid-write is still
// readable up to the last complete frame).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

// Magic marks an encrypted cast file.
const Magic = "TTEC1\n"

// KeySize is the AES-256 key length in bytes.
const KeySize = 32

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
	r        io.Reader
	gcm      cipher.AEAD
	buf      []byte
	frameIdx int
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

// ErrAuthentication is returned when a frame's GCM authentication tag does not
// match, indicating corruption or tampering. It is distinct from io.EOF so
// callers can surface data integrity failures rather than silently stopping.
var ErrAuthentication = fmt.Errorf("crypto: authentication failed — recording may be corrupt or tampered")

func (d *decReader) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		var lenbuf [4]byte
		if _, err := io.ReadFull(d.r, lenbuf[:]); err != nil {
			return 0, io.EOF // clean end or truncated trailing length: stop
		}
		flen := binary.BigEndian.Uint32(lenbuf[:])
		if int(flen) < d.gcm.NonceSize() {
			// Frame length smaller than a nonce: file is truncated at this frame.
			return 0, io.EOF
		}
		const maxFrameSize = 1 << 20 // 1 MiB — largest expected PTY write chunk
		if flen > maxFrameSize {
			return 0, fmt.Errorf("crypto: frame %d: length %d exceeds maximum %d — corrupt file", d.frameIdx, flen, maxFrameSize)
		}
		frame := make([]byte, flen)
		if _, err := io.ReadFull(d.r, frame); err != nil {
			return 0, io.EOF // truncated trailing frame: stop at last complete one
		}
		ns := d.gcm.NonceSize()
		pt, err := d.gcm.Open(nil, frame[:ns], frame[ns:], nil)
		if err != nil {
			// A complete frame that fails authentication is corruption or tampering.
			return 0, fmt.Errorf("crypto: frame %d: %w", d.frameIdx, ErrAuthentication)
		}
		d.frameIdx++
		d.buf = pt
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}
