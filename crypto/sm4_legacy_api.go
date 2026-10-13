package crypto

import (
	"context"
	"github.com/Laisky/errors/v2"
)

// EncryptBySm4CbcBaisc encrypts plaintext under the caller's raw SM4 key and IV
// and returns the historical HMAC-SHA256(key, ciphertext). It runs in-process;
// key bytes never reach child-process arguments. The misspelled name is retained.
//
// Deprecated: The historical MAC does not cover the IV. Use
// EncryptBySm4CbcEnvelope for new protocols, or explicitly adopt
// EncryptBySm4CbcBasicV1 and its paired verifier. Coordinate both endpoints;
// changing the MAC format is not a transparent replacement.
func (t *Tongsuo) EncryptBySm4CbcBaisc(ctx context.Context, key, plaintext, iv []byte) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, errors.Wrap(err, "context done")
	}
	if err := validateSm4KeyAndIV(key, iv); err != nil {
		return nil, nil, errors.Wrap(err, "validate sm4 arguments")
	}
	ct, err := sm4CbcEncrypt(key, iv, plaintext)
	if err != nil {
		return nil, nil, errors.Wrap(err, "encrypt")
	}
	return ct, sm4HMAC(key, ct), nil
}

// DecryptBySm4CbcBaisc preserves the original raw-key, ciphertext-only MAC
// contract. An empty MAC skips integrity verification, as in historical releases.
// Malformed input fails without plaintext; no subprocess is invoked.
//
// Deprecated: Authenticate legacy ciphertext and IV externally. Use
// DecryptBySm4CbcEnvelope with its matching writer for new protocols, or opt into
// DecryptBySm4CbcBasicV1. Never retry this API after a new-format MAC failure.
func (t *Tongsuo) DecryptBySm4CbcBaisc(ctx context.Context, key, ciphertext, iv, tag []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if err := validateSm4DecryptArgs(key, ciphertext, iv, tag); err != nil {
		return nil, errors.Wrap(err, "validate sm4 arguments")
	}
	if len(tag) != 0 {
		return t.DecryptBySm4CbcBaiscLegacy(ctx, key, ciphertext, iv, tag)
	}
	plain, err := sm4CbcDecrypt(key, iv, ciphertext)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}
	return plain, nil
}

// EncryptBySm4Cbc returns the historical IV || ciphertext || HMAC(key,ciphertext)
// format. Empty plaintext produces one padded block. Existing readers remain compatible.
//
// Deprecated: This format does not authenticate its IV. Use
// EncryptBySm4CbcEnvelope after explicitly migrating the reader and data format.
func (t *Tongsuo) EncryptBySm4Cbc(ctx context.Context, key, plain []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	iv, err := Salt(sm4IVSize)
	if err != nil {
		return nil, errors.Wrap(err, "generate iv")
	}
	ct, tag, err := t.EncryptBySm4CbcBaisc(ctx, key, plain, iv)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt by sm4 basic")
	}
	out := make([]byte, 0, len(iv)+len(ct)+len(tag))
	out = append(out, iv...)
	out = append(out, ct...)
	out = append(out, tag...)
	return out, nil
}

// DecryptBySm4Cbc reads the historical combined format without autodetecting any
// new envelope. Authentication errors return no plaintext.
//
// Deprecated: The legacy IV is unauthenticated. Use DecryptBySm4CbcEnvelope for
// explicitly migrated data; do not fall back to this method after its failure.
func (t *Tongsuo) DecryptBySm4Cbc(ctx context.Context, key, combined []byte) ([]byte, error) {
	return t.DecryptBySm4CbcLegacy(ctx, key, combined)
}
