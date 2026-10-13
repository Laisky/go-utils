package crypto

import (
	"bytes"
	"context"
	"encoding/pem"

	"github.com/Laisky/errors/v2"
)

// NewPrikeyWithPassword returns the historical OpenSSL encrypted SEC1
// "EC PRIVATE KEY" PEM, including Proc-Type and DEK-Info headers. Passwords
// stay in the child environment, never argv, and diagnostic stderr is excluded.
// An empty password or malformed subprocess output returns an error and no key.
//
// Deprecated: Legacy PEM encryption uses weak single-pass password derivation.
// Use NewPrikeyPKCS8WithPassword for explicitly migrated consumers supporting
// PBES2-encrypted PKCS#8. Existing SEC1-only callers retain their wire format.
func (t *Tongsuo) NewPrikeyWithPassword(ctx context.Context, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("password should not be empty")
	}
	key, err := t.NewPrikey(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "generate new private key")
	}
	defer clear(key)
	encrypted, err := t.runCMDWithEnv(ctx, []string{
		"ec", tongsuoFlagIn, tongsuoStdinPath, tongsuoFlagOut, "/dev/stdout",
		"-sm4-cbc", "-passout", "env:_TONGSUO_PASSOUT",
	}, key, []string{"_TONGSUO_PASSOUT=" + password})
	if err != nil {
		return nil, errors.Wrap(err, "encrypt private key")
	}
	block, rest := pem.Decode(encrypted)
	if block == nil || block.Type != "EC PRIVATE KEY" || block.Headers["Proc-Type"] != "4,ENCRYPTED" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("encrypt private key: invalid encrypted SEC1 output")
	}
	return encrypted, nil
}

// SignX509CRL signs a DER CRL and returns the historical "X509 CRL" PEM
// representation. It retains all input, signature and field-preservation checks
// of SignX509CRLDER; only the final encoding differs. Errors return no output.
//
// Deprecated: Use SignX509CRLDER when DER output is required. This existing
// method intentionally retains PEM output for existing consumers.
func (t *Tongsuo) SignX509CRL(ctx context.Context, crlDER, privateKeyPEM []byte) ([]byte, error) {
	signed, err := t.SignX509CRLDER(ctx, crlDER, privateKeyPEM)
	if err != nil {
		return nil, errors.Wrap(err, "sign CRL")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: signed}), nil
}
