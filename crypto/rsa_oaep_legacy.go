package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"

	"github.com/Laisky/errors/v2"
)

// RSADecryptByOAEPLegacy decodes the historical sequence of independent RSA-OAEP/SHA-256 blocks for explicit
// offline migration.
// The caller MUST authenticate the complete original ciphertext with a trusted
// external MAC, signature or equivalent integrity record before calling this.
// Reordered, duplicated and deleted whole blocks cannot be detected by this format.
// Empty input and partial blocks are rejected; failures return no partial data.
//
// Deprecated: This format has no whole-message integrity. Use RSADecryptByOAEPEnvelope
// for new authenticated protocols, and never use this function as an automatic fallback.
func RSADecryptByOAEPLegacy(prikey *rsa.PrivateKey, encrypted []byte) ([]byte, error) {
	if err := validateRSAPrivateKey(prikey); err != nil {
		return nil, errors.WithStack(err)
	}
	if prikey.N.BitLen() < 1024 || len(encrypted) == 0 || len(encrypted)%prikey.Size() != 0 {
		return nil, errors.New("invalid legacy RSA-OAEP block sequence")
	}
	var plaintext []byte
	for offset := 0; offset < len(encrypted); offset += prikey.Size() {
		part, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, prikey, encrypted[offset:offset+prikey.Size()], nil)
		if err != nil {
			clear(plaintext)
			return nil, errors.Wrap(err, "decrypt legacy OAEP block")
		}
		plaintext = append(plaintext, part...)
		clear(part)
	}
	return plaintext, nil
}
