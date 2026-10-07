package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/Laisky/errors/v2"
)

// AesCtrStreamEncrypt encrypts the input stream using raw AES in CTR mode.
//
// This is a LOW-LEVEL, CONFIDENTIALITY-ONLY primitive. The returned reader
// yields `IV(16 bytes) || CTR(plaintext)` and carries no tag, MAC, length or
// end marker. An attacker who can modify the stored or transported bytes can
// flip arbitrary plaintext bits, truncate or extend the stream, and none of
// these changes are detected by AesCtrStreamDecrypt.
//
// External-authentication contract: a caller may only use this function when
// it authenticates the complete serialized output (IV included) with an
// independent mechanism, for example HMAC-SHA256 under a separately derived
// key, and verifies that authenticator before trusting or acting on any
// decrypted byte. Callers that need authenticated streaming encryption must use
// AEADStreamEncrypt instead; callers that hold the whole message in memory
// should use AEADEncrypt.
//
// The key must be 16, 24 or 32 bytes. It returns the encrypting reader, or an
// error when the key is invalid or the random IV cannot be generated.
func AesCtrStreamEncrypt(key []byte, reader io.Reader) (io.Reader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "create aes cipher")
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, errors.Wrap(err, "generate iv")
	}

	stream := cipher.NewCTR(block, iv)

	// For empty input we still emit the IV so the stream is decodable.
	return io.MultiReader(
		bytes.NewReader(iv),
		&cipher.StreamReader{
			S: stream,
			R: reader,
		},
	), nil
}

// AesCtrStreamDecrypt decrypts a stream produced by AesCtrStreamEncrypt.
//
// This is a LOW-LEVEL, CONFIDENTIALITY-ONLY primitive. It never returns an
// authentication error: modified ciphertext silently decrypts to modified
// plaintext, and truncated or extended streams decrypt to truncated or extended
// plaintext. Callers must satisfy the external-authentication contract
// described on AesCtrStreamEncrypt (verify an independent MAC over the whole
// serialized stream before trusting any output). It is NOT a replacement for
// the authenticated AesReaderWrapper/AesDecrypt/AEADDecrypt; use
// AEADStreamDecrypt for authenticated streaming decryption.
//
// The key must be 16, 24 or 32 bytes. It returns the decrypting reader, or an
// error when the key is invalid or the IV cannot be read.
func AesCtrStreamDecrypt(key []byte, reader io.Reader) (io.Reader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "create aes cipher")
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(reader, iv); err != nil {
		return nil, errors.Wrap(err, "read iv")
	}

	stream := cipher.NewCTR(block, iv)
	return &cipher.StreamReader{
		S: stream,
		R: reader,
	}, nil
}

// AesReaderWrapper exposes the plaintext of an authenticated AES-GCM message
// (the AesEncrypt/AEADEncrypt format) as an io.Reader. The whole ciphertext is
// verified before any plaintext becomes readable.
//
// Deprecated: use AEADDecrypt for in-memory messages, or the authenticated
// streaming format (AEADStreamEncrypt/AEADStreamDecrypt) for large data. Do NOT
// migrate to AesCtrStreamDecrypt: it is unauthenticated and would silently
// accept tampered ciphertext.
type AesReaderWrapper struct {
	cnt []byte
	idx int
}

// NewAesReaderWrapper reads all of in, authenticates and decrypts it with the
// AES-GCM key, and returns a reader over the verified plaintext. It returns an
// error when reading fails, the key is invalid, or authentication fails.
//
// Deprecated: use AEADDecrypt for in-memory messages, or AEADStreamDecrypt for
// the authenticated streaming format. Do NOT migrate to AesCtrStreamDecrypt,
// which provides no integrity protection.
func NewAesReaderWrapper(in io.Reader, key []byte) (*AesReaderWrapper, error) {
	cipher, err := io.ReadAll(in)
	if err != nil {
		return nil, errors.Wrap(err, "read reader")
	}

	w := new(AesReaderWrapper)
	if w.cnt, err = AesDecrypt(key, cipher); err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	return w, nil
}

// Read copies verified plaintext into p. It returns the number of bytes copied
// and io.EOF once all plaintext has been consumed.
//
// Deprecated: use AEADDecrypt or AEADStreamDecrypt; never AesCtrStreamDecrypt,
// which is unauthenticated.
func (w *AesReaderWrapper) Read(p []byte) (n int, err error) {
	if w.idx == len(w.cnt) {
		return 0, io.EOF
	}

	n = copy(p, w.cnt[w.idx:])
	w.idx += n

	return n, nil
}
