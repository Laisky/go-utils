package crypto

import (
	"context"

	"github.com/Laisky/errors/v2"
)

// EncryptBySm4CbcBaisc preserves raw SM4-CBC/PKCS7 and HMAC-SHA256(key,ciphertext).
// The historical misspelling is retained. Key material never enters subprocess argv.
//
// Deprecated: The MAC omits the IV. Explicitly adopt EncryptBySm4CbcEnvelope or
// EncryptBySm4CbcBasicAuthenticated for IV authentication; formats are not interchangeable.
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

// DecryptBySm4CbcBaisc preserves historical raw CBC and optional ciphertext-only MAC
// verification. A supplied tag is checked before decryption; no partial plaintext is returned.
//
// Deprecated: The IV is unauthenticated, and an empty MAC disables authentication.
// Migrate explicitly to DecryptBySm4CbcEnvelope or DecryptBySm4CbcBasicAuthenticated.
// Never use this function after an authenticated-format failure as a fallback.
func (t *Tongsuo) DecryptBySm4CbcBaisc(ctx context.Context, key, ciphertext, iv, tag []byte) ([]byte, error) {
	if len(tag) != 0 {
		return t.DecryptBySm4CbcBaiscLegacy(ctx, key, ciphertext, iv, tag)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if err := validateSm4DecryptArgs(key, ciphertext, iv, tag); err != nil {
		return nil, errors.Wrap(err, "validate sm4 arguments")
	}
	out, err := sm4CbcDecrypt(key, iv, ciphertext)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}
	return out, nil
}

// EncryptBySm4Cbc preserves iv || SM4-CBC(key,iv,plaintext) || HMAC-SHA256(key,ciphertext).
// It generates a fresh IV and preserves the original format for old readers.
//
// Deprecated: The original format does not authenticate the IV. Use
// EncryptBySm4CbcEnvelope with DecryptBySm4CbcEnvelope through explicit migration.
func (t *Tongsuo) EncryptBySm4Cbc(ctx context.Context, key, plaintext []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if err := validateSm4Key(key); err != nil {
		return nil, errors.Wrap(err, "validate sm4 key")
	}
	iv, err := Salt(sm4IVSize)
	if err != nil {
		return nil, errors.Wrap(err, "generate iv")
	}
	ct, tag, err := t.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt")
	}
	out := append(iv, ct...)
	return append(out, tag...), nil
}

// DecryptBySm4Cbc reads the historical combined SM4 format without format auto-detection.
// Existing ciphertext remains readable through the original public method name.
//
// Deprecated: Its MAC does not authenticate the IV. Use DecryptBySm4CbcEnvelope for
// the versioned format and independently authenticate legacy data before migration.
func (t *Tongsuo) DecryptBySm4Cbc(ctx context.Context, key, combined []byte) ([]byte, error) {
	return t.DecryptBySm4CbcLegacy(ctx, key, combined)
}
