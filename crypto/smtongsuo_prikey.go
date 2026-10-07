package crypto

import (
	"bytes"
	"context"
	"strconv"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	glog "github.com/Laisky/go-utils/v6/log"
)

// NewPrikey generates a new SM2 private key with `tongsuo ecparam -genkey`.
// It returns the command's PEM output, an "EC PARAMETERS" block followed by the
// unencrypted "EC PRIVATE KEY" block, or an error.
//
//	tongsuo ecparam -genkey -name SM2 -out rootca.key
func (t *Tongsuo) NewPrikey(ctx context.Context) (prikeyPem []byte, err error) {
	prikeyPem, err = t.runCMD(ctx, []string{
		"ecparam", "-genkey", "-name", "SM2",
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "generate new private key")
	}

	return prikeyPem, nil
}

// tongsuoPrikeyPBKDF2Iterations is the PBKDF2 iteration count NewPrikeyWithPassword
// uses to derive the key-encryption key, matching the OWASP recommendation of
// 600,000 iterations for PBKDF2-HMAC-SHA256; HMAC-SM3 has the same output size.
const tongsuoPrikeyPBKDF2Iterations = 600000

// NewPrikeyWithPassword generates a new SM2 private key and returns it encrypted
// under password as a PKCS#8 "ENCRYPTED PRIVATE KEY" PEM block.
//
// The encryption is PBES2 (RFC 8018): PBKDF2 with HMAC-SM3, a random 16-byte
// salt and tongsuoPrikeyPBKDF2Iterations iterations, and SM4-CBC. Earlier
// versions emitted the legacy OpenSSL "EC PRIVATE KEY" format with a
// Proc-Type/DEK-Info header, whose key derivation is a single MD5 pass; such
// keys still load with `tongsuo pkey -passin` and can be upgraded with
// `tongsuo pkcs8 -topk8 -v2 sm4-cbc -v2prf hmacWithSM3 -iter 600000`.
//
// The password is passed to tongsuo through the environment, never argv, and
// the unencrypted key only through stdin. It returns the PEM or an error; an
// empty password is rejected.
func (t *Tongsuo) NewPrikeyWithPassword(ctx context.Context, password string) (
	encryptedPrikeyPem []byte, err error) {
	if len(password) == 0 {
		return nil, errors.Errorf("password should not be empty")
	}

	prikeyPem, err := t.NewPrikey(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "generate new private key")
	}

	// Pass password via environment variable instead of command-line argument
	// to avoid exposing it in the process list (ps aux, /proc/*/cmdline).
	encryptedPrikeyPem, err = t.runCMDWithEnv(ctx, []string{
		"pkcs8", "-topk8", tongsuoFlagIn, tongsuoStdinPath,
		"-v2", "sm4-cbc", "-v2prf", "hmacWithSM3",
		"-iter", strconv.Itoa(tongsuoPrikeyPBKDF2Iterations), "-saltlen", "16",
		"-passout", "env:_TONGSUO_PASSOUT",
	}, prikeyPem, []string{"_TONGSUO_PASSOUT=" + password})
	if err != nil {
		return nil, errors.Wrap(err, "encrypt private key")
	}
	if !bytes.HasPrefix(encryptedPrikeyPem, []byte("-----BEGIN ENCRYPTED PRIVATE KEY-----")) {
		return nil, errors.New("encrypt private key: tongsuo did not return a PKCS#8 encrypted private key")
	}

	glog.Shared.Debug("generated password-encrypted sm2 private key",
		zap.String("format", "pkcs8-pbes2"),
		zap.String("kdf", "pbkdf2-hmac-sm3"),
		zap.Int("iterations", tongsuoPrikeyPBKDF2Iterations),
		zap.String("cipher", "sm4-cbc"))
	return encryptedPrikeyPem, nil
}
