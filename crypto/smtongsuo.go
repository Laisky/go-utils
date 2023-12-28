package crypto

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	gutils "github.com/Laisky/go-utils/v4"
	glog "github.com/Laisky/go-utils/v4/log"
)

// Tongsuo is a wrapper of tongsuo executable binary
//
// https://github.com/Tongsuo-Project/Tongsuo
type Tongsuo struct {
	exePath         string
	serialGenerator *DefaultX509CertSerialNumGenerator
}

// NewTongsuo new tongsuo wrapper
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

	// new serial number generator
	if ins.serialGenerator, err = NewDefaultX509CertSerialNumGenerator(); err != nil {
		return nil, errors.Wrap(err, "new serial number generator")
	}

	return ins, nil
}

func (t *Tongsuo) runCMD(ctx context.Context, args []string, stdin []byte) (output []byte, err error) {
	if args, err = gutils.SanitizeCMDArgs(args); err != nil {
		return nil, errors.Wrap(err, "sanitize cmd args")
	}

	//nolint: gosec
	// G204: Subprocess launched with a potential tainted input or cmd arguments
	cmd := exec.CommandContext(ctx, t.exePath, args...)
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

// ShowCertInfo show cert info
func (t *Tongsuo) ShowCertInfo(ctx context.Context, certDer []byte) (output string, err error) {
	out, err := t.runCMD(ctx, []string{"x509", "-inform", "DER", "-text"}, certDer)
	if err != nil {
		return "", errors.Wrap(err, "run cmd to show cert info")
	}

	return string(out), nil
}

// ShowCsrInfo show csr info
func (t *Tongsuo) ShowCsrInfo(ctx context.Context, csrDer []byte) (output string, err error) {
	out, err := t.runCMD(ctx, []string{"req", "-inform", "DER", "-text"}, csrDer)
	if err != nil {
		return "", errors.Wrap(err, "run cmd to show csr info")
	}

	return string(out), nil
}

// NewPrikey generate new sm2 private key
func (t *Tongsuo) NewPrikey(ctx context.Context) (prikeyPem []byte, err error) {
	prikeyPem, err = t.runCMD(ctx, []string{
		"genpkey", "-outform", "PEM", "-algorithm", "SM2",
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "generate new private key")
	}

	return prikeyPem, nil
}

func (t *Tongsuo) removeAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		glog.Shared.Error("remove dir", zap.String("path", path), zap.Error(err))
	}
}

// NewPrikeyAndCert generate new private key and root ca
func (t *Tongsuo) NewPrikeyAndCert(ctx context.Context, opts ...X509CertOption) (prikeyPem, certDer []byte, err error) {
	// new private key
	if prikeyPem, err = t.NewPrikey(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "new private key")
	}

	opt, tpl, err := x509CertOption2Template(opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "X509CertOption2Template")
	}

	opensslConf := X509Cert2OpensslConf(tpl)
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate tem dir")
	}
	defer t.removeAll(dir)

	// write conf
	confPath := filepath.Join(dir, "rootca.cnf")
	if err = os.WriteFile(confPath, opensslConf, 0600); err != nil {
		return nil, nil, errors.Wrap(err, "write openssl conf")
	}

	outCertPemPath := filepath.Join(dir, "rootca.pem")

	// new root ca
	if _, err = t.runCMD(ctx, []string{
		"req", "-outform", "PEM", "-out", outCertPemPath,
		"-key", "/dev/stdin",
		"-set_serial", strconv.Itoa(int(t.serialGenerator.SerialNum())),
		"-days", strconv.Itoa(int(time.Until(opt.notAfter) / time.Hour / 24)),
		"-x509", "-new", "-nodes", "-utf8", "-batch",
		"-sm3", "-sigopt", "sm2-za:no",
		"-copy_extensions", "copyall",
		"-extensions", "v3_ca",
		"-config", confPath,
	}, prikeyPem); err != nil {
		return nil, nil, errors.Wrap(err, "generate new root ca")
	}

	certPem, err := os.ReadFile(outCertPemPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "read root ca")
	}

	if certDer, err = Pem2Der(certPem); err != nil {
		return nil, nil, errors.Wrap(err, "Pem2Der")
	}

	return prikeyPem, certDer, nil
}

// NewX509CSR generate new x509 csr
func (t *Tongsuo) NewX509CSR(ctx context.Context, prikeyPem []byte, opts ...X509CSROption) (csrDer []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate tem dir")
	}
	defer t.removeAll(dir)

	tpl, err := X509CsrOption2Template(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "X509CsrOption2Template")
	}

	opensslConf := X509Csr2OpensslConf(tpl)
	confPath := filepath.Join(dir, "csr.cnf")
	if err = os.WriteFile(confPath, opensslConf, 0600); err != nil {
		return nil, errors.Wrap(err, "write openssl conf")
	}

	outCsrDerPath := filepath.Join(dir, "csr.der")

	if _, err = t.runCMD(ctx, []string{
		"req", "-new", "-outform", "DER", "-out", outCsrDerPath,
		"-key", "/dev/stdin",
		"-sm3", "-sigopt", "sm2-za:no",
		"-config", confPath,
	}, prikeyPem); err != nil {
		return nil, errors.Wrap(err, "generate new csr")
	}

	if csrDer, err = os.ReadFile(outCsrDerPath); err != nil {
		return nil, errors.Wrap(err, "read csr")
	}

	return csrDer, nil
}

// NewX509CertByCSR generate new x509 cert by csr
func (t *Tongsuo) NewX509CertByCSR(ctx context.Context,
	parentCertDer []byte,
	parentPrikeyPem []byte,
	csrDer []byte,
	opts ...SignCSROption) (certDer []byte, err error) {
	opt, opensslConf, err := x509SignCsrOptions2OpensslConf(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "X509SignCsrOptions2OpensslConf")
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate tem dir")
	}
	defer t.removeAll(dir)

	confPath := filepath.Join(dir, "csr.cnf")
	if err = os.WriteFile(confPath, opensslConf, 0600); err != nil {
		return nil, errors.Wrap(err, "write openssl conf")
	}

	parentCertDerPath := filepath.Join(dir, "ca.der")
	if err = os.WriteFile(parentCertDerPath, parentCertDer, 0600); err != nil {
		return nil, errors.Wrap(err, "write parent cert")
	}

	csrDerPath := filepath.Join(dir, "csr.der")
	if err = os.WriteFile(csrDerPath, csrDer, 0600); err != nil {
		return nil, errors.Wrap(err, "write csr")
	}

	outCertDerPath := filepath.Join(dir, "cert.der")

	if _, err = t.runCMD(ctx, []string{
		"x509", "-req", "-outform", "DER", "-out", outCertDerPath,
		"-in", csrDerPath, "-inform", "DER",
		"-CA", parentCertDerPath, "-CAkey", "/dev/stdin", "-CAcreateserial",
		"-days", strconv.Itoa(int(time.Until(opt.notAfter) / time.Hour / 24)),
		"-utf8", "-batch",
		"-sm3", "-sigopt", "sm2-za:no", "-vfyopt", "sm2-za:no",
		"-copy_extensions", "copyall",
		"-extfile", confPath, "-extensions", "v3_ca",
	}, parentPrikeyPem); err != nil {
		return nil, errors.Wrap(err, "sign csr")
	}

	if certDer, err = os.ReadFile(outCertDerPath); err != nil {
		return nil, errors.Wrap(err, "read signed cert")
	}

	return certDer, nil
}

// EncryptBySm4Baisc encrypt by sm4
//
// # Args
//   - key: sm4 key, should be 16 bytes
//   - plaintext: data to be encrypted
//   - iv: sm4 iv, should be 16 bytes
//
// # Returns
//   - ciphertext: sm4 encrypted data
//   - hmac: hmac of ciphertext, 32 bytes
func (t *Tongsuo) EncryptBySm4Baisc(ctx context.Context,
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
		return nil, nil, errors.Wrap(err, "generate tem dir")
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

	if hmac, err = HMACSha256(key, ciphertext); err != nil {
		return nil, nil, errors.Wrap(err, "calculate hmac")
	}

	return ciphertext, hmac, nil
}

// DecryptBySm4Baisc decrypt by sm4
//
// # Args
//   - key: sm4 key
//   - ciphertext: sm4 encrypted data
//   - iv: sm4 iv
//   - hmac: if not nil, will check ciphertext's integrity by hmac
func (t *Tongsuo) DecryptBySm4Baisc(ctx context.Context,
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
		if expectedHmac, err := HMACSha256(key, ciphertext); err != nil {
			return nil, errors.Wrap(err, "calculate hmac")
		} else if !bytes.Equal(hmac, expectedHmac) {
			return nil, errors.Errorf("hmac not match")
		}
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate tem dir")
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

// EncryptBySm4 encrypt by sm4, should be decrypted by `DecryptBySm4` only
func (t *Tongsuo) EncryptBySm4(ctx context.Context, key, plaintext []byte) (combinedCipher []byte, err error) {
	iv, err := Salt(16)
	if err != nil {
		return nil, errors.Wrap(err, "generate iv")
	}

	cipher, hmac, err := t.EncryptBySm4Baisc(ctx, key, plaintext, iv)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt by sm4 basic")
	}

	combinedCipher = make([]byte, 0, len(iv)+len(cipher)+len(hmac))
	combinedCipher = append(combinedCipher, iv...)
	combinedCipher = append(combinedCipher, cipher...)
	combinedCipher = append(combinedCipher, hmac...)

	return combinedCipher, nil
}

// DecryptBySm4 decrypt by sm4, should be encrypted by `EncryptBySm4` only
func (t *Tongsuo) DecryptBySm4(ctx context.Context, key, combinedCipher []byte) (plaintext []byte, err error) {
	if len(combinedCipher) <= 48 {
		return nil, errors.Errorf("invalid combined cipher")
	}

	iv := combinedCipher[:16]
	cipher := combinedCipher[16 : len(combinedCipher)-32]
	hmac := combinedCipher[len(combinedCipher)-32:]

	return t.DecryptBySm4Baisc(ctx, key, cipher, iv, hmac)
}
