// Package crypto is a collection of cryptographic algorithms and protocols, providing
// hash functions, block and stream ciphers, public key cryptography and authentication.
// It also includes a cryptographically secure pseudo-random number generator.
//
// x.509
//
// This package provides many useful functions for x.509 certificate.
// you can build a PKI system with this package.
// including parsing and verification. it can be used to parse x.509 certificates,
// create x.509 certificate chains, verify x.509 certificate chains,
// and parse x.509 certificate revocation lists.
package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"math/big"

	"github.com/Laisky/errors/v2"
)

// FormatBig2Hex format big to hex string
func FormatBig2Hex(b *big.Int) string {
	return b.Text(16)
}

// ParseHex2Big parse hex string to big
func ParseHex2Big(hex string) (b *big.Int, ok bool) {
	b = new(big.Int)
	return b.SetString(hex, 16)
}

// FormatBig2Base64 format big to base64 string
func FormatBig2Base64(b *big.Int) string {
	return base64.URLEncoding.EncodeToString(b.Bytes())
}

// ParseBase642Big parse base64 string to big
func ParseBase642Big(raw string) (*big.Int, error) {
	bb, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	b := new(big.Int)
	b.SetBytes(bb)
	return b, nil
}

var (
	// RSAEncrypt encrypt by RSAEncryptByPKCS1v15, for compatibility
	//
	// Deprecated: use RSAEncryptByPKCS1v15 or RsaEncryptByOAEP instead
	RSAEncrypt = RSAEncryptByPKCS1v15
	// RSADecrypt decrypt by RSADecryptByPKCS1v15, for compatibility
	//
	// Deprecated: use RSADecryptByPKCS1v15 or RSADecryptByOAEP instead
	RSADecrypt = RSADecryptByPKCS1v15
)

// RSAEncryptByPKCS1v15 encrypt by PKCS1v15
//
// This is not a deterministic encryption scheme,
// it will return different ciphertexts each time
// even if the same plaintext is encrypted multiple times.
func RSAEncryptByPKCS1v15(pubkey *rsa.PublicKey, plain []byte) (cipher []byte, err error) {
	if pubkey == nil {
		return nil, errors.Errorf("public key is nil")
	}

	// PKCS#1 v1.5 reserves 11 bytes of padding, so the modulus must be able to
	// hold at least one plaintext byte. Guard before sizing the chunk buffer,
	// otherwise `make` is called with a negative length and panics on tiny keys.
	chunkSize := pubkey.Size() - 11
	if chunkSize <= 0 {
		return nil, errors.Errorf("rsa public key too small (%d bytes) for PKCS1v15", pubkey.Size())
	}

	chunk := make([]byte, chunkSize) // will padding 11 bytes
	reader := bytes.NewReader(plain)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		cipherChunk, err := rsa.EncryptPKCS1v15(rand.Reader, pubkey, chunk[:n])
		if err != nil {
			return nil, errors.Wrap(err, "encrypt chunk")
		}

		cipher = append(cipher, cipherChunk...)
	}

	return cipher, nil
}

// RSADecryptByPKCS1v15 decrypt by rsa PKCS1v15
//
// only accept cipher encrypted by RSAEncrypt
func RSADecryptByPKCS1v15(prikey *rsa.PrivateKey, cipher []byte) (plain []byte, err error) {
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

		plainChunk, err := rsa.DecryptPKCS1v15(rand.Reader, prikey, chunk[:n])
		if err != nil {
			return nil, errors.Wrap(err, "decrypt chunk")
		}

		plain = append(plain, plainChunk...)
	}

	return plain, nil
}

// RSAEncryptByOAEP encrypts by OAEP with SHA256
//
// This is not a deterministic encryption scheme,
// it will return different ciphertexts each time
// even if the same plaintext is encrypted multiple times.
func RSAEncryptByOAEP(pubkey *rsa.PublicKey, plain []byte) (cipher []byte, err error) {
	if pubkey == nil || pubkey.N == nil || pubkey.N.Sign() <= 0 || pubkey.N.Bit(0) == 0 ||
		pubkey.E < 3 || pubkey.E > (1<<31)-1 || pubkey.E%2 == 0 {
		return nil, errors.New("invalid RSA public key")
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

// RSADecryptByOAEP decrypt by OAEP with SHA256
func RSADecryptByOAEP(prikey *rsa.PrivateKey, cipher []byte) (plain []byte, err error) {
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

// ConstantTimeStringEqual hashes candidate to a fixed-size digest before
// comparing it with expectedHash. The candidate parameter is untrusted user
// input, expectedHash is the SHA-256 digest of the configured secret, and the
// return value reports equality without leaking candidate length through
// subtle.ConstantTimeCompare's length check.
func ConstantTimeStringEqual(candidate string, expectedHash [sha256.Size]byte) bool {
	candidateHash := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(candidateHash[:], expectedHash[:]) == 1
}
