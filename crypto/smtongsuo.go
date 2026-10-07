package crypto

import (
	"bytes"
	"context"
	"crypto"
	cryptohmac "crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
)

// Tongsuo is a wrapper of tongsuo executable binary
//
// https://github.com/Tongsuo-Project/Tongsuo
type Tongsuo struct {
	exePath         string
	serialGenerator *DefaultX509CertSerialNumGenerator
	// exactValidity reports whether both `x509` and `req` accept
	// -not_before/-not_after, detected once by NewTongsuo.
	exactValidity bool
}

// NewTongsuo new tongsuo wrapper
//
// Notice, only support
//   - github.com/tongsuo-project/tongsuo-go-sdk v0.0.0-20231225081335-82a881b9b3d3
//   - https://github.com/Tongsuo-Project/Tongsuo 8.4.0-pre3
//
// #Args
//   - exePath: path of tongsuo executable binary
func NewTongsuo(exePath string) (ins *Tongsuo, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ins = &Tongsuo{exePath: exePath}

	// check tongsuo executable binary
	if out, err := ins.runCMD(ctx, []string{"version"}, nil); err != nil {
		return nil, errors.Wrapf(err, "run `%s version` failed", exePath)
	} else if !strings.Contains(string(out), "Tongsuo") {
		return nil, errors.Errorf("only support Tongsuo")
	}

	// detect exact validity support once; older binaries fall back to a
	// conservative whole-day encoding that never extends NotAfter
	ins.exactValidity = ins.probeExactValidity(context.Background())

	// new serial number generator
	if ins.serialGenerator, err = NewDefaultX509CertSerialNumGenerator(); err != nil {
		return nil, errors.Wrap(err, "new serial number generator")
	}

	return ins, nil
}

func (t *Tongsuo) runCMD(ctx context.Context, args []string, stdin []byte) (
	output []byte, err error) {
	return t.runCMDWithEnv(ctx, args, stdin, nil)
}

// runCMDWithEnv runs a tongsuo command with optional extra environment variables.
//
// Use extraEnv to pass sensitive values (keys, passwords) via process environment
// instead of command-line arguments, which would be visible in the process list.
func (t *Tongsuo) runCMDWithEnv(ctx context.Context, args []string, stdin []byte, extraEnv []string) (
	output []byte, err error) {
	if args, err = gutils.SanitizeCMDArgs(args); err != nil {
		return nil, errors.Wrap(err, "sanitize cmd args")
	}

	//nolint: gosec
	// G204: Subprocess launched with a potential tainted input or cmd arguments
	cmd := exec.CommandContext(ctx, t.exePath, args...)
	if len(extraEnv) != 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if len(stdin) != 0 {
		var stdinBuf bytes.Buffer
		stdinBuf.Write(stdin)
		cmd.Stdin = &stdinBuf
	}

	if output, err = cmd.CombinedOutput(); err != nil {
		return nil, errors.Wrapf(err, "run cmd failed, got %s", output)
	}

	return output, nil
}

// NewPrikey generate new sm2 private key
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

// NewPrikeyWithPassword generate new sm2 private key with password
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
		"ec", "-in", "/dev/stdin", "-out", "/dev/stdout",
		"-sm4-cbc", "-passout", "env:_TONGSUO_PASSOUT",
	}, prikeyPem, []string{"_TONGSUO_PASSOUT=" + password})
	if err != nil {
		return nil, errors.Wrap(err, "encrypt private key")
	}

	return encryptedPrikeyPem, nil
}

func (t *Tongsuo) removeAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		glog.Shared.Error("remove dir", zap.String("path", path), zap.Error(err))
	}
}

// Prikey2Pubkey convert private key to public key
func (t *Tongsuo) Prikey2Pubkey(ctx context.Context, prikeyPem []byte) (
	pubkeyPem []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	pubkeyPath := filepath.Join(dir, "pubkey")
	if _, err = t.runCMD(ctx,
		[]string{
			"ec", "-in", "/dev/stdin", "-pubout", "-out", pubkeyPath,
		}, prikeyPem); err != nil {
		return nil, errors.Wrap(err, "convert private key to public key")
	}

	if pubkeyPem, err = os.ReadFile(pubkeyPath); err != nil {
		return nil, errors.Wrap(err, "read public key")
	}

	return pubkeyPem, nil
}

// NewPrikeyAndCert generate new private key and root ca
func (t *Tongsuo) NewPrikeyAndCert(ctx context.Context, opts ...X509CertOption) (
	prikeyPem, certDer []byte, err error) {
	// new private key
	if prikeyPem, err = t.NewPrikey(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "new private key")
	}

	certDer, err = t.NewX509Cert(ctx, prikeyPem, opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new root ca")
	}

	return prikeyPem, certDer, nil
}

// NewX509CertByCSR signs csrDer with the parent CA certificate and private
// key through the tongsuo binary and returns the issued certificate DER.
//
// The validity window is validated before issuance (zero, past, empty and
// inverted windows are rejected) and encoded exactly with -not_before and
// -not_after when the binary supports them, or as conservative whole days
// otherwise; NotAfter is never extended. The issued certificate is parsed
// before it is returned and must lie within the requested validity and carry
// exactly the requested extended key usages (x509.ExtKeyUsageAny stays the
// single anyExtendedKeyUsage OID); otherwise an error is returned and no
// certificate bytes are exposed.
func (t *Tongsuo) NewX509CertByCSR(ctx context.Context,
	parentCertDer []byte,
	parentPrikeyPem []byte,
	csrDer []byte,
	opts ...SignCSROption) (certDer []byte, err error) {
	if certDer, err = t.signX509CSR(ctx, parentCertDer, parentPrikeyPem, csrDer, opts...); err != nil {
		return nil, errors.Wrap(err, "sign csr")
	}

	return certDer, nil
}

// EncryptBySm4CbcBaisc encrypt by sm4
//
// # Security Warning
//
// The SM4 key and IV are passed as command-line arguments to the tongsuo binary,
// which makes them visible to other local users via /proc/*/cmdline or `ps aux`.
// This is a limitation of OpenSSL's `-K`/`-iv` flags which do not support
// environment variable or file-based input. Use this function only in
// environments where local process visibility is not a concern.
//
// # Args
//   - key: sm4 key, should be 16 bytes
//   - plaintext: data to be encrypted
//   - iv: sm4 iv, should be 16 bytes
//
// # Returns
//   - ciphertext: sm4 encrypted data
//   - hmac: hmac of ciphertext, 32 bytes
func (t *Tongsuo) EncryptBySm4CbcBaisc(ctx context.Context,
	key, plaintext, iv []byte) (ciphertext, hmac []byte, err error) {
	if len(key) != 16 {
		return nil, nil, errors.Errorf("key should be 16 bytes")
	}
	if len(iv) != 16 {
		return nil, nil, errors.Errorf("iv should be 16 bytes")
	}
	if len(hmac) != 0 && len(hmac) != 32 {
		return nil, nil, errors.Errorf("hmac should be 0 or 32 bytes")
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	cipherPath := filepath.Join(dir, "cipher")
	if _, err = t.runCMD(ctx, []string{
		"enc", "-sm4-cbc", "-e",
		"-in", "/dev/stdin", "-out", cipherPath,
		"-K", hex.EncodeToString(key), "-iv", hex.EncodeToString(iv),
	}, plaintext); err != nil {
		return nil, nil, errors.Wrap(err, "encrypt")
	}

	if ciphertext, err = os.ReadFile(cipherPath); err != nil {
		return nil, nil, errors.Wrap(err, "read cipher")
	}

	if hmac, err = HMACSha256(key, bytes.NewReader(ciphertext)); err != nil {
		return nil, nil, errors.Wrap(err, "calculate hmac")
	}

	return ciphertext, hmac, nil
}

// DecryptBySm4CbcBaisc decrypt by sm4
//
// # Security Warning
//
// The SM4 key and IV are passed as command-line arguments to the tongsuo binary,
// which makes them visible to other local users via /proc/*/cmdline or `ps aux`.
// See EncryptBySm4CbcBaisc for details.
//
// # Args
//   - key: sm4 key
//   - ciphertext: sm4 encrypted data
//   - iv: sm4 iv
//   - hmac: if not nil, will check ciphertext's integrity by hmac
func (t *Tongsuo) DecryptBySm4CbcBaisc(ctx context.Context,
	key, ciphertext, iv, hmac []byte) (plaintext []byte, err error) {
	if len(key) != 16 {
		return nil, errors.Errorf("key should be 16 bytes")
	}
	if len(iv) != 16 {
		return nil, errors.Errorf("iv should be 16 bytes")
	}
	if len(hmac) != 0 && len(hmac) != 32 {
		return nil, errors.Errorf("hmac should be 0 or 32 bytes")
	}

	if len(hmac) != 0 { // check hmac
		if expectedHmac, err := HMACSha256(key, bytes.NewReader(ciphertext)); err != nil {
			return nil, errors.Wrap(err, "calculate hmac")
		} else if !cryptohmac.Equal(hmac, expectedHmac) {
			return nil, errors.Errorf("hmac not match")
		}
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	cipherPath := filepath.Join(dir, "cipher")
	if err = os.WriteFile(cipherPath, ciphertext, 0600); err != nil {
		return nil, errors.Wrap(err, "write cipher")
	}

	if plaintext, err = t.runCMD(ctx, []string{
		"enc", "-sm4-cbc", "-d",
		"-in", cipherPath, "-out", "/dev/stdout",
		"-K", hex.EncodeToString(key), "-iv", hex.EncodeToString(iv),
	}, ciphertext); err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	return plaintext, nil
}

// EncryptBySm4Cbc encrypt by sm4, should be decrypted by `DecryptBySm4` only
func (t *Tongsuo) EncryptBySm4Cbc(ctx context.Context, key, plaintext []byte) (
	combinedCipher []byte, err error) {
	iv, err := Salt(16)
	if err != nil {
		return nil, errors.Wrap(err, "generate iv")
	}

	cipher, hmac, err := t.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt by sm4 basic")
	}

	combinedCipher = make([]byte, 0, len(iv)+len(cipher)+len(hmac))
	combinedCipher = append(combinedCipher, iv...)
	combinedCipher = append(combinedCipher, cipher...)
	combinedCipher = append(combinedCipher, hmac...)

	return combinedCipher, nil
}

// DecryptBySm4Cbc decrypt by sm4, should be encrypted by `EncryptBySm4` only
func (t *Tongsuo) DecryptBySm4Cbc(ctx context.Context, key, combinedCipher []byte) (
	plaintext []byte, err error) {
	if len(combinedCipher) <= 48 {
		return nil, errors.Errorf("invalid combined cipher")
	}

	iv := combinedCipher[:16]
	cipher := combinedCipher[16 : len(combinedCipher)-32]
	hmac := combinedCipher[len(combinedCipher)-32:]

	return t.DecryptBySm4CbcBaisc(ctx, key, cipher, iv, hmac)
}

var (
	reX509Subject = regexp.MustCompile(`(?s)Subject: ([\S ]+)`)
	reX509Sans    = regexp.MustCompile(`(?m)X509v3 Subject Alternative Name: ?\n +(.+)\b`)
)

// ParseCsr2Opts parse csr to opts
func (t *Tongsuo) ParseCsr2Opts(ctx context.Context, csrDer []byte) ([]X509CSROption, error) {
	csrinfo, err := t.ShowCsrInfo(ctx, csrDer)
	if err != nil {
		return nil, errors.Wrap(err, "show csr info")
	}

	var opts []X509CSROption

	// extract subjects
	// Subject: C = CN, ST = Shanghai, L = Shanghai, O = BBT, CN = Intermediate CA
	matched := reX509Subject.FindStringSubmatch(csrinfo)
	if len(matched) != 2 {
		return nil, errors.Errorf("invalid csr info")
	}
	sbjs := strings.Split(matched[1], ", ")
	for _, sbj := range sbjs {
		kv := strings.Split(sbj, " = ")
		if len(kv) != 2 {
			return nil, errors.Errorf("invalid subject info %q", sbj)
		}

		switch kv[0] {
		case "C":
			opts = append(opts, WithX509CSRCountry(kv[1]))
		case "ST":
			opts = append(opts, WithX509CSRProvince(kv[1]))
		case "L":
			opts = append(opts, WithX509CSRLocality(kv[1]))
		case "O":
			opts = append(opts, WithX509CSROrganization(kv[1]))
		case "CN":
			opts = append(opts, WithX509CSRCommonName(kv[1]))
		}
	}

	// extract SANs
	// X509v3 Subject Alternative Name:
	//     DNS:www.example.com, DNS:www.example.net, DNS:www.example.origin
	matched = reX509Sans.FindStringSubmatch(csrinfo)
	if len(matched) == 2 {
		sans := strings.Split(matched[1], ", ")
		for _, san := range sans {
			kv := strings.Split(san, ":")
			if len(kv) != 2 {
				return nil, errors.Errorf("invalid csr info %q", san)
			}

			opts = append(opts, WithX509CSRSANS(kv[1]))
		}
	}

	return opts, nil
}

// CloneX509Csr generat a cloned csr with different private key
//
// # Args
//   - prikeyPem: new private key for cloned csr
//   - originCsrDer: origin csr
func (t *Tongsuo) CloneX509Csr(ctx context.Context,
	prikeyPem []byte, originCsrDer []byte) (clonedCsrDer []byte, err error) {
	opts, err := t.ParseCsr2Opts(ctx, originCsrDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse csr to opts")
	}

	// generate new csr
	clonedCsrDer, err = t.NewX509CSR(ctx, prikeyPem, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "generate cloned csr")
	}

	return clonedCsrDer, nil
}

// SignBySm2Sm3 sign by sm2 sm3
//
// https://www.yuque.com/tsdoc/ts/ewh6xg7qlddxlec2#rehkK
func (t *Tongsuo) SignBySm2Sm3(ctx context.Context,
	parentPrikeyPem []byte, content []byte) (signature []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	contentPath := filepath.Join(dir, "input")
	if err = os.WriteFile(contentPath, content, 0600); err != nil {
		return nil, errors.Wrap(err, "write input")
	}

	outputPath := filepath.Join(dir, "output")

	_, err = t.runCMD(ctx,
		[]string{
			"dgst", "-sm3", "-sign", "/dev/stdin",
			"-out", outputPath,
			contentPath,
		},
		parentPrikeyPem,
	)
	if err != nil {
		return nil, errors.Wrap(err, "sign by sm2 sm3")
	}

	if signature, err = os.ReadFile(outputPath); err != nil {
		return nil, errors.Wrap(err, "read signature")
	}

	return signature, nil
}

// VerifyCertsChain verify certs chain
//
// # Args
//   - leafCert: leaf cert in PEM
//   - intermediates: intermediate certs in PEM
//   - trustRoots: trust roots in PEM
func (t *Tongsuo) VerifyCertsChain(ctx context.Context,
	leafCertPem, intermediatesPem, trustRootsPem []byte) error {
	if len(leafCertPem) == 0 {
		return errors.Errorf("leaf cert should not be empty")
	}
	if len(trustRootsPem) == 0 {
		return errors.Errorf("trust roots should not be empty")
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	// write leaf cert
	leafCertPath := filepath.Join(dir, "leaf.crt")
	if err = os.WriteFile(leafCertPath, leafCertPem, 0600); err != nil {
		return errors.Wrap(err, "write leaf cert")
	}

	// write root ca
	rootCaPath := filepath.Join(dir, "rootca.crt")
	if err = os.WriteFile(rootCaPath, trustRootsPem, 0600); err != nil {
		return errors.Wrap(err, "write root ca")
	}

	// write intermediate certs
	interCaPath := filepath.Join(dir, "intermediate.crt")
	if err = os.WriteFile(interCaPath, intermediatesPem, 0600); err != nil {
		return errors.Wrap(err, "write intermediate certs")
	}

	cmd := []string{
		"verify", "-CAfile", rootCaPath,
	}
	if len(intermediatesPem) != 0 {
		cmd = append(cmd, []string{"-untrusted", interCaPath}...)
	}
	cmd = append(cmd, leafCertPath)

	_, err = t.runCMD(ctx, cmd, nil)
	if err != nil {
		return errors.Wrap(err, "cannot verify certs chain")
	}

	return nil
}

// VerifyBySm2Sm3 verify by sm2 sm3
//
// https://www.yuque.com/tsdoc/ts/ewh6xg7qlddxlec2#rehkK
func (t *Tongsuo) VerifyBySm2Sm3(ctx context.Context,
	pubkeyPem, signature, content []byte) error {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	contentPath := filepath.Join(dir, "input")
	if err = os.WriteFile(contentPath, content, 0600); err != nil {
		return errors.Wrap(err, "write input")
	}

	pubkeyPath := filepath.Join(dir, "pubkey")
	if err = os.WriteFile(pubkeyPath, pubkeyPem, 0600); err != nil {
		return errors.Wrap(err, "write pubkey")
	}

	signaturePath := filepath.Join(dir, "signature")
	if err = os.WriteFile(signaturePath, signature, 0600); err != nil {
		return errors.Wrap(err, "write signature")
	}

	_, err = t.runCMD(ctx,
		[]string{
			"dgst", "-sm3", "-verify", pubkeyPath,
			"-signature", signaturePath,
			contentPath,
		},
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "verify by sm2 sm3")
	}

	return nil
}

// HashBySm3 hash by sm3
func (t *Tongsuo) HashBySm3(ctx context.Context, content []byte) (hash []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	// contentPath := filepath.Join(dir, "input")
	// if err = os.WriteFile(contentPath, content, 0600); err != nil {
	// 	return nil, errors.Wrap(err, "write input")
	// }

	outputPath := filepath.Join(dir, "output")

	_, err = t.runCMD(ctx,
		[]string{
			"dgst", "-sm3", "-binary",
			"-out", outputPath,
		},
		content,
	)
	if err != nil {
		return nil, errors.Wrap(err, "hash by sm3")
	}

	if hash, err = os.ReadFile(outputPath); err != nil {
		return nil, errors.Wrap(err, "read hash")
	}

	return hash, nil
}

// GetPubkeyFromCertPem get pubkey from cert pem
func (t *Tongsuo) GetPubkeyFromCertPem(ctx context.Context, certPem []byte) (pubkeyPem []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	certPath := filepath.Join(dir, "cert.crt")
	if err = os.WriteFile(certPath, certPem, 0600); err != nil {
		return nil, errors.Wrap(err, "write cert")
	}

	pubkeyPath := filepath.Join(dir, "pubkey")
	if _, err = t.runCMD(ctx, []string{
		"x509", "-pubkey", "-noout",
		"-in", certPath, "-out", pubkeyPath,
	}, nil); err != nil {
		return nil, errors.Wrap(err, "get pubkey from cert")
	}

	if pubkeyPem, err = os.ReadFile(pubkeyPath); err != nil {
		return nil, errors.Wrap(err, "read pubkey")
	}

	return pubkeyPem, nil
}

// EncryptBySm2 encrypt by sm2 public key
func (t *Tongsuo) EncryptBySm2(ctx context.Context,
	pubkeyPem []byte, data []byte) (cipher []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	dataPath := filepath.Join(dir, "data")
	if err = os.WriteFile(dataPath, data, 0600); err != nil {
		return nil, errors.Wrap(err, "write data")
	}

	pubkeyPath := filepath.Join(dir, "pubkey")
	if err = os.WriteFile(pubkeyPath, pubkeyPem, 0600); err != nil {
		return nil, errors.Wrap(err, "write pubkey")
	}

	cipherPath := filepath.Join(dir, "cipher")
	if _, err = t.runCMD(ctx, []string{
		"pkeyutl", "-inkey", pubkeyPath, "-pubin", "-encrypt",
		"-in", dataPath, "-out", cipherPath,
	}, nil); err != nil {
		return nil, errors.Wrap(err, "encrypt by sm2")
	}

	if cipher, err = os.ReadFile(cipherPath); err != nil {
		return nil, errors.Wrap(err, "read cipher")
	}

	return cipher, nil
}

// DecryptBySm2 decrypt by sm2 private key
func (t *Tongsuo) DecryptBySm2(ctx context.Context,
	prikeyPem []byte, cipher []byte) (data []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	cipherPath := filepath.Join(dir, "cipher")
	if err = os.WriteFile(cipherPath, cipher, 0600); err != nil {
		return nil, errors.Wrap(err, "write cipher")
	}

	prikeyPath := filepath.Join(dir, "prikey")
	if err = os.WriteFile(prikeyPath, prikeyPem, 0600); err != nil {
		return nil, errors.Wrap(err, "write prikey")
	}

	dataPath := filepath.Join(dir, "data")
	if _, err = t.runCMD(ctx, []string{
		"pkeyutl", "-inkey", prikeyPath, "-decrypt",
		"-in", cipherPath, "-out", dataPath,
	}, nil); err != nil {
		return nil, errors.Wrap(err, "decrypt by sm2")
	}

	if data, err = os.ReadFile(dataPath); err != nil {
		return nil, errors.Wrap(err, "read data")
	}

	return data, nil
}

// SignX509CRL sign x509 crl by ca private key
func (t *Tongsuo) SignX509CRL(ctx context.Context,
	CrlDer []byte,
	PrikeyPem []byte,
) (signedCrlDer []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	// write crl file
	crlPath := filepath.Join(dir, "crl")
	crlPem := CRLDer2Pem(CrlDer)
	if err = os.WriteFile(crlPath, crlPem, 0600); err != nil {
		return nil, errors.Wrap(err, "write crl")
	}

	// sign crl
	signedCrlPath := filepath.Join(dir, "signed_crl")
	if _, err = t.runCMD(ctx, []string{
		"crl", "-in", crlPath, "-out", signedCrlPath, "-signkey", "/dev/stdin",
	}, PrikeyPem); err != nil {
		return nil, errors.Wrap(err, "sign crl")
	}

	signedCrlDer, err = os.ReadFile(signedCrlPath)
	if err != nil {
		return nil, errors.Wrap(err, "read signed crl")
	}

	return signedCrlDer, nil
}

// PrivateKey get private key
func (t *Tongsuo) PrivateKey(prikeyPem []byte) (crypto.PrivateKey, error) {
	return &TongsuoPriKey{ts: t, pem: prikeyPem}, nil
}

// TongsuoPubkey tongsuo public key
type TongsuoPubkey struct {
	pem []byte
}

// Equal compare two public keys
func (tpub *TongsuoPubkey) Equal(x crypto.PublicKey) bool {
	xpub, ok := x.(*TongsuoPubkey)
	if !ok {
		return false
	}

	return bytes.Equal(tpub.pem, xpub.pem)
}

// TongsuoPriKey tongsuo private key
type TongsuoPriKey struct {
	ts  *Tongsuo
	pem []byte
}

// Sign sign by private key
func (t *TongsuoPriKey) Sign(_ io.Reader, digest []byte,
	opts crypto.SignerOpts) (signature []byte, err error) {
	if opts.HashFunc() == 0 {
		hasher := sha256.New()
		hasher.Write(digest)
		digest = hasher.Sum(nil)
	}

	return t.ts.SignBySm2Sm3(context.Background(), t.pem, digest)
}

// Public get public key
func (t *TongsuoPriKey) Public() crypto.PublicKey {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	pubkeyPem, err := t.ts.Prikey2Pubkey(ctx, t.pem)
	if err != nil {
		return nil
	}

	return &TongsuoPubkey{pem: pubkeyPem}
}

// Decrypt decrypt by private key
func (t *TongsuoPriKey) Decrypt(_ io.Reader, msg []byte,
	_ crypto.DecrypterOpts) (plaintext []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	return t.ts.DecryptBySm2(ctx, t.pem, msg)
}
