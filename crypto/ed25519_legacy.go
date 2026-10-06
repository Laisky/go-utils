package crypto

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"io"

	"github.com/Laisky/errors/v2"
)

// SignByEd25519LegacySHA512 signs SHA512(reader) with plain Ed25519, preserving
// the historical WithSHA512 wire format. Use only for explicitly selected legacy
// protocols, with keys kept separate from plain Ed25519 signing services. New
// prehash protocols should use SignByEd25519ph and a distinct format identifier.
func SignByEd25519LegacySHA512(prikey ed25519.PrivateKey, reader io.Reader) ([]byte, error) {
	if len(prikey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid Ed25519 private key length")
	}
	if reader == nil {
		return nil, errors.New("legacy Ed25519 reader must not be nil")
	}
	hasher := sha512.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return nil, errors.Wrap(err, "read legacy Ed25519 message")
	}
	signature, err := prikey.Sign(nil, hasher.Sum(nil), crypto.Hash(0))
	if err != nil {
		return nil, errors.Wrap(err, "sign legacy Ed25519 message")
	}
	return signature, nil
}

// VerifyByEd25519LegacySHA512 verifies only the historical plain-Ed25519-over-
// SHA512 format. The protocol must be selected externally; no Ed25519ph fallback
// is attempted. Malformed key/signature lengths and nil readers return errors.
func VerifyByEd25519LegacySHA512(pubkey ed25519.PublicKey, reader io.Reader, signature []byte) error {
	if len(pubkey) != ed25519.PublicKeySize {
		return errors.New("invalid Ed25519 public key length")
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("invalid signature: legacy Ed25519 length")
	}
	if reader == nil {
		return errors.New("legacy Ed25519 reader must not be nil")
	}
	hasher := sha512.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return errors.Wrap(err, "read legacy Ed25519 message")
	}
	if !ed25519.Verify(pubkey, hasher.Sum(nil), signature) {
		return errors.New("invalid signature")
	}
	return nil
}
