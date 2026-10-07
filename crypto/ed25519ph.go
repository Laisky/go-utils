package crypto

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"io"

	"github.com/Laisky/errors/v2"
)

// SignByEd25519ph streams reader into SHA-512 and returns an RFC 8032 Ed25519ph
// signature using the supplied private key and an empty context string.
// This is a distinct protocol from legacy plain Ed25519 over a SHA-512 digest.
func SignByEd25519ph(prikey ed25519.PrivateKey, reader io.Reader) ([]byte, error) {
	if len(prikey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid Ed25519 private key length")
	}
	if reader == nil {
		return nil, errors.New("Ed25519ph reader must not be nil")
	}
	hasher := sha512.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return nil, errors.Wrap(err, "read Ed25519ph message")
	}
	signature, err := prikey.Sign(nil, hasher.Sum(nil), &ed25519.Options{Hash: crypto.SHA512})
	if err != nil {
		return nil, errors.Wrap(err, "sign Ed25519ph message")
	}
	return signature, nil
}

// VerifyByEd25519ph verifies an RFC 8032 Ed25519ph signature with empty context.
// Malformed keys return errors rather than panicking. No legacy fallback is tried.
func VerifyByEd25519ph(pubkey ed25519.PublicKey, reader io.Reader, signature []byte) error {
	if len(pubkey) != ed25519.PublicKeySize {
		return errors.New("invalid Ed25519 public key length")
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("invalid Ed25519ph signature length")
	}
	if reader == nil {
		return errors.New("Ed25519ph reader must not be nil")
	}
	hasher := sha512.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return errors.Wrap(err, "read Ed25519ph message")
	}
	options := &ed25519.Options{Hash: crypto.SHA512}
	if err := ed25519.VerifyWithOptions(pubkey, hasher.Sum(nil), signature, options); err != nil {
		return errors.Wrap(err, "verify Ed25519ph signature")
	}
	return nil
}
