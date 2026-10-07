package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"
	"math"

	"github.com/Laisky/errors/v2"
)

// Authenticated streaming format (version 1), a STREAM construction in the
// style of age and Tink AES-GCM-HKDF streaming:
//
//	header  = magic "GUSTREAM" (8) || version 0x01 (1) || chunk size uint32 BE (4) || salt (32)
//	subkey  = HKDF-SHA256(ikm=key, salt=salt, info=label || header), 32 bytes (AES-256)
//	chunk i = AES-256-GCM(subkey, nonce_i, plaintext_i), each plaintext_i is exactly
//	          chunk size bytes except the final chunk, which holds 0..chunk size bytes
//	nonce_i = 11-byte big-endian i || 1-byte flag (0x01 for the final chunk, else 0x00)
//
// The whole header is bound into the subkey derivation, so any header change
// yields a different subkey and the first chunk fails authentication.
const (
	// AEADStreamDefaultChunkSize is the default plaintext bytes per authenticated chunk.
	AEADStreamDefaultChunkSize = 64 * 1024
	// AEADStreamMinChunkSize is the smallest accepted plaintext chunk size.
	AEADStreamMinChunkSize = 64
	// AEADStreamMaxChunkSize is the largest accepted plaintext chunk size. It
	// bounds the per-stream buffer allocated for an untrusted header.
	AEADStreamMaxChunkSize = 4 * 1024 * 1024

	aeadStreamMagic     = "GUSTREAM"
	aeadStreamVersion1  = byte(1)
	aeadStreamSaltLen   = 32
	aeadStreamHeaderLen = len(aeadStreamMagic) + 1 + 4 + aeadStreamSaltLen
	aeadStreamSubkeyLen = 32
	aeadStreamFinalFlag = byte(1)
	aeadStreamKeyLabel  = "github.com/Laisky/go-utils/crypto AEADStream v1 AES-256-GCM chunk key"
)

// aeadStreamOption holds the configuration of AEADStreamEncrypt.
type aeadStreamOption struct {
	chunkSize int
}

// AEADStreamOption configures AEADStreamEncrypt.
type AEADStreamOption func(*aeadStreamOption) error

// WithAEADStreamChunkSize sets the plaintext bytes per authenticated chunk.
// The size must be within [AEADStreamMinChunkSize, AEADStreamMaxChunkSize];
// it returns an option that fails otherwise. The decoder reads the size from
// the authenticated header, so no matching decrypt option is needed.
func WithAEADStreamChunkSize(size int) AEADStreamOption {
	return func(o *aeadStreamOption) error {
		if size < AEADStreamMinChunkSize || size > AEADStreamMaxChunkSize {
			return errors.Errorf("chunk size must be within [%d, %d], got %d",
				AEADStreamMinChunkSize, AEADStreamMaxChunkSize, size)
		}

		o.chunkSize = size
		return nil
	}
}

// AEADStreamEncrypt returns a reader that yields the authenticated streaming
// encryption of r. Use it instead of the unauthenticated AesCtrStreamEncrypt.
//
// The key must be 16, 24 or 32 bytes of uniformly random key material (use
// EncryptByPassword for human passwords). Every stream draws a fresh random
// 32-byte salt, and the per-stream AES-256-GCM subkey (always 32 bytes) is
// derived from key and salt with HKDF-SHA256, so a key may encrypt many
// streams. Plaintext is processed in chunks (default AEADStreamDefaultChunkSize)
// so memory use is bounded by the chunk size.
//
// It returns the ciphertext reader, or an error for an invalid key, invalid
// option, nil reader, or randomness failure. Read errors from r are returned by
// the ciphertext reader; a stream cut short by such an error has no final
// chunk and is rejected by AEADStreamDecrypt. The reader is not safe for
// concurrent use.
func AEADStreamEncrypt(key []byte, r io.Reader, opts ...AEADStreamOption) (io.Reader, error) {
	if r == nil {
		return nil, errors.New("plaintext reader must not be nil")
	}
	if err := checkAESKeyLen(key); err != nil {
		return nil, errors.WithStack(err)
	}

	opt := aeadStreamOption{chunkSize: AEADStreamDefaultChunkSize}
	for _, f := range opts {
		if err := f(&opt); err != nil {
			return nil, errors.Wrap(err, "apply AEAD stream option")
		}
	}

	salt, err := Salt(aeadStreamSaltLen)
	if err != nil {
		return nil, errors.Wrap(err, "generate stream salt")
	}

	header := make([]byte, 0, aeadStreamHeaderLen)
	header = append(header, aeadStreamMagic...)
	header = append(header, aeadStreamVersion1)
	// The chunk size is bounded by option validation, so the conversion is exact.
	header = binary.BigEndian.AppendUint32(header, uint32(opt.chunkSize)) //nolint:gosec // bounded above.
	header = append(header, salt...)

	aead, err := newAEADStreamCipher(key, header)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &aeadStreamEncrypter{
		src:       r,
		aead:      aead,
		chunkSize: opt.chunkSize,
		buf:       make([]byte, opt.chunkSize+1),
		sealed:    make([]byte, 0, opt.chunkSize+AesGcmTagLen),
		out:       header,
	}, nil
}

// AEADStreamDecrypt reads and validates the stream header from r and returns a
// reader that yields the authenticated plaintext of a stream produced by
// AEADStreamEncrypt with the same key.
//
// Each chunk is authenticated before any of its plaintext is released. Chunk
// tampering, reordering, duplication, truncation (including a stream that ends
// without its final chunk), trailing data after the final chunk, header
// tampering and a wrong key are all reported as errors by Read.
//
// IMPORTANT: plaintext of earlier, individually authenticated chunks is
// released before the end of the stream is verified. The output as a whole is
// trustworthy only once Read has returned io.EOF without any prior error; a
// caller must discard (or not act on) everything read when any error occurs.
//
// It returns an error before reading any chunk when the key length is not 16,
// 24 or 32 bytes, the header cannot be read, or the header carries an unknown
// magic, unsupported version or out-of-range chunk size. The reader is not safe
// for concurrent use.
func AEADStreamDecrypt(key []byte, r io.Reader) (io.Reader, error) {
	if r == nil {
		return nil, errors.New("ciphertext reader must not be nil")
	}
	if err := checkAESKeyLen(key); err != nil {
		return nil, errors.WithStack(err)
	}

	header := make([]byte, aeadStreamHeaderLen)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, errors.Wrap(err, "read AEAD stream header")
	}

	chunkSize, err := parseAEADStreamHeader(header)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	aead, err := newAEADStreamCipher(key, header)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &aeadStreamDecrypter{
		src:       r,
		aead:      aead,
		chunkSize: chunkSize,
		buf:       make([]byte, chunkSize+AesGcmTagLen+1),
		plain:     make([]byte, 0, chunkSize),
	}, nil
}

// checkAESKeyLen returns an error unless key is 16, 24 or 32 bytes long.
// The error never contains key material.
func checkAESKeyLen(key []byte) error {
	switch len(key) {
	case 16, 24, 32:
		return nil
	default:
		return errors.Errorf("invalid AES key length %d, want 16, 24 or 32 bytes", len(key))
	}
}

// parseAEADStreamHeader validates an untrusted stream header and returns its
// chunk size. It checks magic, version and chunk-size bounds before the caller
// allocates any chunk buffer, and returns an error for any invalid field.
func parseAEADStreamHeader(header []byte) (int, error) {
	if len(header) != aeadStreamHeaderLen || string(header[:len(aeadStreamMagic)]) != aeadStreamMagic {
		return 0, errors.New("not an AEAD stream: bad magic")
	}

	off := len(aeadStreamMagic)
	if header[off] != aeadStreamVersion1 {
		return 0, errors.Errorf("unsupported AEAD stream version %d", header[off])
	}

	size := binary.BigEndian.Uint32(header[off+1 : off+5])
	if size < AEADStreamMinChunkSize || size > AEADStreamMaxChunkSize {
		return 0, errors.Errorf("AEAD stream chunk size %d out of range [%d, %d]",
			size, AEADStreamMinChunkSize, AEADStreamMaxChunkSize)
	}

	return int(size), nil
}

// newAEADStreamCipher derives the per-stream AES-256-GCM subkey from key and the
// complete header (whose trailing bytes are the salt) and returns the AEAD.
// It returns an error when derivation or cipher construction fails.
func newAEADStreamCipher(key, header []byte) (cipher.AEAD, error) {
	salt := header[len(header)-aeadStreamSaltLen:]
	info := make([]byte, 0, len(aeadStreamKeyLabel)+len(header))
	info = append(info, aeadStreamKeyLabel...)
	info = append(info, header...)

	subkey := make([]byte, aeadStreamSubkeyLen)
	defer clear(subkey)
	if err := HKDFWithSHA256(key, salt, info, [][]byte{subkey}); err != nil {
		return nil, errors.Wrap(err, "derive AEAD stream subkey")
	}

	block, err := aes.NewCipher(subkey)
	if err != nil {
		return nil, errors.Wrap(err, "new aes cipher")
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "new gcm")
	}

	return aead, nil
}

// aeadStreamNonce writes the nonce for chunk counter and the final flag into
// dst (12 bytes): an 11-byte big-endian counter followed by the flag byte.
func aeadStreamNonce(dst []byte, counter uint64, final bool) {
	clear(dst[:3])
	binary.BigEndian.PutUint64(dst[3:11], counter)
	dst[11] = 0
	if final {
		dst[11] = aeadStreamFinalFlag
	}
}

// readChunk fills buf[pending:] from src and reports the new pending length and
// whether src reached EOF before buf was full. Non-EOF read errors are returned.
func readChunk(src io.Reader, buf []byte, pending int) (int, bool, error) {
	n, err := io.ReadFull(src, buf[pending:])
	pending += n
	switch {
	case err == nil:
		return pending, false, nil
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return pending, true, nil
	default:
		return pending, false, errors.WithStack(err)
	}
}

// aeadStreamEncrypter is the io.Reader returned by AEADStreamEncrypt.
type aeadStreamEncrypter struct {
	src       io.Reader
	aead      cipher.AEAD
	chunkSize int
	// buf holds up to chunkSize+1 plaintext bytes; the extra byte is a
	// lookahead that proves the current chunk is not the final one.
	buf     []byte
	pending int
	sealed  []byte
	out     []byte
	nonce   [12]byte
	counter uint64
	done    bool
	err     error
}

// Read implements io.Reader. It returns ciphertext bytes, io.EOF after the
// final chunk, or the sticky error that stopped encryption.
func (e *aeadStreamEncrypter) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for len(e.out) == 0 {
		if e.err != nil {
			return 0, e.err
		}
		if e.done {
			return 0, io.EOF
		}
		if err := e.sealNext(); err != nil {
			e.err = err
			return 0, err
		}
	}

	n := copy(p, e.out)
	e.out = e.out[n:]
	return n, nil
}

// sealNext reads the next plaintext chunk and seals it into e.out. It returns
// an error for source read failures or counter exhaustion.
func (e *aeadStreamEncrypter) sealNext() error {
	pending, eof, err := readChunk(e.src, e.buf, e.pending)
	if err != nil {
		return errors.Wrap(err, "read plaintext")
	}
	e.pending = pending

	if e.counter == math.MaxUint64 {
		return errors.New("AEAD stream chunk counter exhausted")
	}

	chunkLen := e.chunkSize
	if eof {
		chunkLen = e.pending
	}

	aeadStreamNonce(e.nonce[:], e.counter, eof)
	e.sealed = e.aead.Seal(e.sealed[:0], e.nonce[:], e.buf[:chunkLen], nil)
	e.out = e.sealed
	e.counter++

	if eof {
		e.done = true
		e.pending = 0
		clear(e.buf)
		return nil
	}

	// Carry the lookahead byte into the next chunk.
	e.buf[0] = e.buf[chunkLen]
	e.pending = 1
	return nil
}

// aeadStreamDecrypter is the io.Reader returned by AEADStreamDecrypt.
type aeadStreamDecrypter struct {
	src       io.Reader
	aead      cipher.AEAD
	chunkSize int
	// buf holds up to chunkSize+tag+1 ciphertext bytes; the extra byte is a
	// lookahead that decides whether the current chunk must be the final one.
	buf     []byte
	pending int
	plain   []byte
	out     []byte
	nonce   [12]byte
	counter uint64
	done    bool
	err     error
}

// Read implements io.Reader. It returns authenticated plaintext bytes, io.EOF
// after the authenticated final chunk, or the sticky error that stopped
// decryption.
func (d *aeadStreamDecrypter) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for len(d.out) == 0 {
		if d.err != nil {
			return 0, d.err
		}
		if d.done {
			return 0, io.EOF
		}
		if err := d.openNext(); err != nil {
			d.err = err
			return 0, err
		}
	}

	n := copy(p, d.out)
	d.out = d.out[n:]
	return n, nil
}

// openNext reads and authenticates the next ciphertext chunk into d.out. It
// returns an error for read failures, truncation, authentication failures,
// non-canonical encodings, or counter exhaustion.
func (d *aeadStreamDecrypter) openNext() error {
	pending, eof, err := readChunk(d.src, d.buf, d.pending)
	if err != nil {
		return errors.Wrap(err, "read ciphertext")
	}
	d.pending = pending

	if d.counter == math.MaxUint64 {
		return errors.New("AEAD stream chunk counter exhausted")
	}

	ctLen := d.chunkSize + AesGcmTagLen
	if eof {
		ctLen = d.pending
		if ctLen < AesGcmTagLen {
			return errors.Errorf("AEAD stream truncated: final chunk missing after chunk %d", d.counter)
		}
	}

	aeadStreamNonce(d.nonce[:], d.counter, eof)
	plain, err := d.aead.Open(d.plain[:0], d.nonce[:], d.buf[:ctLen], nil)
	if err != nil {
		return errors.Wrapf(err,
			"authenticate AEAD stream chunk %d (final=%t): wrong key, or data truncated, reordered or tampered",
			d.counter, eof)
	}
	if eof && len(plain) == 0 && d.counter != 0 {
		return errors.New("AEAD stream has a non-canonical empty final chunk")
	}

	d.plain = plain
	d.out = plain
	d.counter++

	if eof {
		d.done = true
		d.pending = 0
		return nil
	}

	d.buf[0] = d.buf[ctLen]
	d.pending = 1
	return nil
}
