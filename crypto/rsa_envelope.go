package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"

	"github.com/Laisky/errors/v2"
)

const (
	rsaEnvelopeMagic     = "GURSAENV"
	rsaEnvelopeVersion   = 1
	rsaEnvelopeHeaderLen = len(rsaEnvelopeMagic) + 1 + 2
	rsaEnvelopeNonceLen  = 12
	rsaEnvelopeKeyLen    = 32
	rsaEnvelopeTagLen    = 16
	rsaEnvelopeLabel     = "go-utils RSA-OAEP AES-256-GCM envelope v1"
)

// ErrInvalidRSAEnvelope reports malformed or unauthenticated ciphertext without exposing partial plaintext or a primitive-specific decryption reason.
var ErrInvalidRSAEnvelope = errors.New("invalid or unauthenticated RSA envelope")

// RSAEncryptByOAEP encrypts one complete message with a fresh AES-256-GCM key wrapped by RSA-OAEP/SHA-256.
// The versioned header, wrapped key and nonce are authenticated as associated data.
// Empty plaintext still produces an authenticated envelope. This format replaces
// independent RSA blocks; see docs/security/rsa-oaep-envelope.md for migration.
// This provides message integrity, not sender authentication or replay protection.
func RSAEncryptByOAEP(pubkey *rsa.PublicKey, plain []byte) ([]byte, error) {
	if err := validateRSAPublicKey(pubkey); err != nil {
		return nil, errors.WithStack(err)
	}
	if pubkey.N.BitLen() < 1024 || pubkey.Size() > 65535 {
		return nil, errors.New("RSA-OAEP envelope requires a modulus of at least 1024 bits and at most 65535 bytes")
	}
	overhead := rsaEnvelopeHeaderLen + pubkey.Size() + rsaEnvelopeNonceLen + rsaEnvelopeTagLen
	if len(plain) > int(^uint(0)>>1)-overhead || uint64(len(plain)) > uint64((1<<32)-2)*aes.BlockSize {
		return nil, errors.New("RSA-OAEP envelope plaintext is too large")
	}
	key := make([]byte, rsaEnvelopeKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, errors.Wrap(err, "generate envelope key")
	}
	defer clear(key)
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubkey, key, []byte(rsaEnvelopeLabel))
	if err != nil {
		return nil, errors.Wrap(err, "wrap envelope key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "create envelope cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "create envelope AEAD")
	}
	prefixLen := rsaEnvelopeHeaderLen + len(wrapped) + rsaEnvelopeNonceLen
	envelope := make([]byte, prefixLen, overhead+len(plain))
	copy(envelope, rsaEnvelopeMagic)
	envelope[len(rsaEnvelopeMagic)] = rsaEnvelopeVersion
	binary.BigEndian.PutUint16(envelope[len(rsaEnvelopeMagic)+1:rsaEnvelopeHeaderLen], uint16(len(wrapped)))
	copy(envelope[rsaEnvelopeHeaderLen:], wrapped)
	nonce := envelope[prefixLen-rsaEnvelopeNonceLen : prefixLen]
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "generate envelope nonce")
	}
	// Seal appends after the immutable AAD prefix. The output region and AAD
	// do not overlap, even though their backing allocation is shared.
	return aead.Seal(envelope, nonce, plain, envelope[:prefixLen]), nil
}

// RSADecryptByOAEP authenticates a versioned RSA-OAEP/AES-GCM envelope and returns the complete plaintext or an error.
// It never falls back to unauthenticated legacy blocks, including single blocks,
// and never returns partial plaintext on failure. Empty ciphertext is invalid.
func RSADecryptByOAEP(prikey *rsa.PrivateKey, encrypted []byte) ([]byte, error) {
	if err := validateRSAPrivateKey(prikey); err != nil {
		return nil, errors.WithStack(err)
	}
	if len(encrypted) < rsaEnvelopeHeaderLen || !bytes.Equal(encrypted[:len(rsaEnvelopeMagic)], []byte(rsaEnvelopeMagic)) || encrypted[len(rsaEnvelopeMagic)] != rsaEnvelopeVersion {
		return nil, errors.WithStack(ErrInvalidRSAEnvelope)
	}
	wrappedLen := int(binary.BigEndian.Uint16(encrypted[len(rsaEnvelopeMagic)+1 : rsaEnvelopeHeaderLen]))
	if prikey.N.BitLen() < 1024 || wrappedLen != prikey.Size() {
		return nil, errors.WithStack(ErrInvalidRSAEnvelope)
	}
	prefixLen := rsaEnvelopeHeaderLen + wrappedLen + rsaEnvelopeNonceLen
	if len(encrypted) < prefixLen+rsaEnvelopeTagLen {
		return nil, errors.WithStack(ErrInvalidRSAEnvelope)
	}
	key, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, prikey, encrypted[rsaEnvelopeHeaderLen:rsaEnvelopeHeaderLen+wrappedLen], []byte(rsaEnvelopeLabel))
	if err != nil {
		return nil, errors.WithStack(ErrInvalidRSAEnvelope)
	}
	defer clear(key)
	if len(key) != rsaEnvelopeKeyLen {
		return nil, errors.WithStack(ErrInvalidRSAEnvelope)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "create envelope cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "create envelope AEAD")
	}
	nonce := encrypted[prefixLen-rsaEnvelopeNonceLen : prefixLen]
	plaintext, err := aead.Open(nil, nonce, encrypted[prefixLen:], encrypted[:prefixLen])
	if err != nil {
		return nil, errors.WithStack(ErrInvalidRSAEnvelope)
	}
	return plaintext, nil
}
