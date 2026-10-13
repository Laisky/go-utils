package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"io"

	"github.com/Laisky/errors/v2"
)

// RSAEncryptByOAEP encrypts plaintext as the historical concatenation of independent
// RSA-OAEP/SHA-256 blocks with a nil label. Empty plaintext produces nil ciphertext.
// The original wire format and validation behavior are retained for existing callers.
// Repeated encryption produces different ciphertext even for the same plaintext.
//
// Deprecated: Independent blocks do not authenticate message order or completeness.
// Keep this API only for compatibility with an externally authenticated legacy protocol.
// Use RSAEncryptByOAEPEnvelope with RSADecryptByOAEPEnvelope for new protocols;
// adopting the envelope requires an explicit format migration, not a drop-in rename.
func RSAEncryptByOAEP(pubkey *rsa.PublicKey, plain []byte) (cipher []byte, err error) {
	if err = validateRSAPublicKey(pubkey); err != nil {
		return nil, errors.WithStack(err)
	}
	if pubkey.N.BitLen() < 1024 {
		return nil, errors.New("RSA-OAEP requires at least a 1024-bit modulus")
	}
	chunkSize := pubkey.Size() - 2*sha256.Size - 2
	if chunkSize <= 0 {
		return nil, errors.New("RSA key has no positive SHA-256 OAEP payload capacity")
	}

	// Slice a positive-sized chunk directly, avoiding both a key-sized temporary
	// allocation and a zero-byte reader loop. Reject weak keys even for empty
	// plaintext; the standard library also validates each nonempty operation.
	for len(plain) > 0 {
		n := min(chunkSize, len(plain))
		cipherChunk, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubkey, plain[:n], nil)
		if err != nil {
			return nil, errors.Wrap(err, "encrypt chunk")
		}
		cipher = append(cipher, cipherChunk...)
		plain = plain[n:]
	}
	return cipher, nil
}

// RSADecryptByOAEP decrypts the historical concatenation of RSA-OAEP/SHA-256 blocks
// with a nil label and returns plaintext or an error. Empty ciphertext returns nil, nil.
// Complete block deletion, duplication and reordering cannot be detected by this format.
// Errors return no partial plaintext; the original validation and error context are retained.
//
// Deprecated: This format has no whole-message integrity. Retained for existing callers;
// authenticate the complete ciphertext externally before accepting it across a trust boundary.
// Use RSADecryptByOAEPEnvelope for the explicitly versioned authenticated format.
// Do not use this function as a fallback after envelope authentication fails.
func RSADecryptByOAEP(prikey *rsa.PrivateKey, cipher []byte) (plain []byte, err error) {
	if err = validateRSAPrivateKey(prikey); err != nil {
		return nil, errors.WithStack(err)
	}
	chunk := make([]byte, prikey.Size())
	reader := bytes.NewReader(cipher)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		plainChunk, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, prikey, chunk[:n], nil)
		if err != nil {
			return nil, errors.Wrap(err, "decrypt chunk")
		}

		plain = append(plain, plainChunk...)
	}

	return plain, nil
}
