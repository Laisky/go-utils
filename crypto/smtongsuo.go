package crypto

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func (t *Tongsuo) runCMD(ctx context.Context, args []string, stdin []byte) (
	output []byte, err error) {
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

// OpensslCertificateOutput output of `openssl x509 -inform DER -text`
type OpensslCertificateOutput struct {
	Raw                 []byte
	SerialNumber        string
	NotBefore, NotAfter time.Time
}

// ShowCertInfo show cert info
//
// # Raw
//
//			Version: 3 (0x2)
//			Serial Number: 17108345756590001 (0x3cc7f327841fb1)
//			Signature Algorithm: SM2-with-SM3
//			Issuer: CN = test-common-name, O = test org
//			Validity
//				Not Before: Mar 19 07:49:35 2024 GMT
//				Not After : Mar 25 07:49:35 2024 GMT
//			Subject: CN = test-common-name, O = test org
//			Subject Public Key Info:
//				Public Key Algorithm: id-ecPublicKey
//					Public-Key: (256 bit)
//					pub:
//						04:31:66:dd:ef:4e:31:29:fd:4b:b1:a1:66:0b:c9:
//						81:9f:6f:a4:e1:bd:44:24:6a:a8:93:62:0b:85:be:
//						0e:56:14:76:ab:56:0d:7c:cc:26:77:47:d0:fe:77:
//						38:31:ab:3d:b8:01:60:96:ae:07:72:e4:3d:df:4c:
//						9d:02:98:9f:d3
//					ASN1 OID: SM2
//			X509v3 extensions:
//				X509v3 Basic Constraints: critical
//					CA:TRUE
//				X509v3 Key Usage:
//					Certificate Sign, CRL Sign
//				X509v3 Subject Key Identifier:
//					AF:9A:33:37:3F:DE:3E:DD:77:61:A1:C8:3F:D5:0C:39:F0:D6:A6:7B
//				X509v3 Authority Key Identifier:
//					AF:9A:33:37:3F:DE:3E:DD:77:61:A1:C8:3F:D5:0C:39:F0:D6:A6:7B
//				X509v3 Certificate Policies:
//					Policy: 1.3.6.1.4.1.59936.1.1.3
//		Signature Algorithm: SM2-with-SM3
//		Signature Value:
//			30:45:02:21:00:a8:a6:db:d5:8c:b4:d2:58:ff:1e:1f:9d:c1:
//			e7:0b:eb:ba:4b:50:99:2c:c4:b9:3b:50:9d:6f:5f:1f:32:40:
//			17:02:20:38:91:fb:16:41:80:52:d8:28:f8:ee:34:0f:f9:ab:
//			c5:c8:1a:1f:31:d9:05:13:04:12:4d:0c:3d:fd:52:fe:51
//	-----BEGIN CERTIFICATE-----
//	MIIByzCCAXGgAwIBAgIHPMfzJ4QfsTAKBggqgRzPVQGDdTAuMRkwFwYDVQQDDBB0
//	ZXN0LWNvbW1vbi1uYW1lMREwDwYDVQQKDAh0ZXN0IG9yZzAeFw0yNDAzMTkwNzQ5
//	MzVaFw0yNDAzMjUwNzQ5MzVaMC4xGTAXBgNVBAMMEHRlc3QtY29tbW9uLW5hbWUx
//	ETAPBgNVBAoMCHRlc3Qgb3JnMFkwEwYHKoZIzj0CAQYIKoEcz1UBgi0DQgAEMWbd
//	704xKf1LsaFmC8mBn2+k4b1EJGqok2ILhb4OVhR2q1YNfMwmd0fQ/nc4Mas9uAFg
//	lq4HcuQ930ydApif06N6MHgwDwYDVR0TAQH/BAUwAwEB/zALBgNVHQ8EBAMCAQYw
//	HQYDVR0OBBYEFK+aMzc/3j7dd2GhyD/VDDnw1qZ7MB8GA1UdIwQYMBaAFK+aMzc/
//	3j7dd2GhyD/VDDnw1qZ7MBgGA1UdIAQRMA8wDQYLKwYBBAGD1CABAQMwCgYIKoEc
//	z1UBg3UDSAAwRQIhAKim29WMtNJY/x4fncHnC+u6S1CZLMS5O1Cdb18fMkAXAiA4
//	kfsWQYBS2Cj47jQP+avFyBofMdkFEwQSTQw9/VL+UQ==
//	-----END CERTIFICATE-----
func (t *Tongsuo) ShowCertInfo(ctx context.Context, certDer []byte) (certInfo OpensslCertificateOutput, err error) {
	certInfo.Raw, err = t.runCMD(ctx,
		[]string{"x509", "-inform", "DER", "-text"},
		certDer)
	if err != nil {
		return certInfo, errors.Wrap(err, "run cmd to show cert info")
	}

	// parse serial no
	if matched := regexp.MustCompile(`(?m)Serial Number: ?\n? +([^ ]+)\b`).
		FindAllSubmatch(certInfo.Raw, 1); len(matched) != 1 || len(matched[0]) != 2 {
		return certInfo, errors.Errorf("cert info should contain serial number")
	} else {
		certInfo.SerialNumber = strings.ReplaceAll(string(matched[0][1]), ":", "")
	}

	// parse not before and not after
	if matched := regexp.MustCompile(`(?m)Not Before: ?\n? +(.+)\b`).
		FindAllSubmatch(certInfo.Raw, 1); len(matched) != 1 || len(matched[0]) != 2 {
		return certInfo, errors.Errorf("cert info should contain not before")
	} else {
		certInfo.NotBefore, err = time.Parse("Jan 2 15:04:05 2006 MST", string(matched[0][1]))
		if err != nil {
			return certInfo, errors.Wrap(err, "parse not before")
		}
	}
	if matched := regexp.MustCompile(`(?m)Not After : ?\n? +(.+)\b`).
		FindAllSubmatch(certInfo.Raw, 1); len(matched) != 1 || len(matched[0]) != 2 {
		return certInfo, errors.Errorf("cert info should contain not after")
	} else {
		certInfo.NotAfter, err = time.Parse("Jan 2 15:04:05 2006 MST", string(matched[0][1]))
		if err != nil {
			return certInfo, errors.Wrap(err, "parse not after")
		}
	}

	return certInfo, nil
}

// ShowCsrInfo show csr info
func (t *Tongsuo) ShowCsrInfo(ctx context.Context, csrDer []byte) (
	output string, err error) {
	out, err := t.runCMD(ctx, []string{"req", "-inform", "DER", "-text"}, csrDer)
	if err != nil {
		return "", errors.Wrap(err, "run cmd to show csr info")
	}

	return string(out), nil
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

	encryptedPrikeyPem, err = t.runCMD(ctx, []string{
		"ec", "-in", "/dev/stdin", "-out", "/dev/stdout",
		"-sm4-cbc", "-passout", "pass:" + password,
	}, prikeyPem)
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

// NewX509Cert generate new x509 cert
//
//	tongsuo req -out rootca.crt -outform PEM -key rootca.key \
//	    -set_serial 123456 \
//	    -days 3650 -x509 -new -nodes -utf8 -batch \
//	    -sm3 \
//	    -copy_extensions copyall \
//	    -extensions v3_ca \
//	    -config rootca.cnf
func (t *Tongsuo) NewX509Cert(ctx context.Context,
	prikeyPem []byte, opts ...X509CertOption) (certDer []byte, err error) {
	opt, tpl, err := x509CertOption2Template(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "X509CertOption2Template")
	}

	opensslConf := X509Cert2OpensslConf(tpl)
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	// write conf
	confPath := filepath.Join(dir, "rootca.cnf")
	if err = os.WriteFile(confPath, opensslConf, 0600); err != nil {
		return nil, errors.Wrap(err, "write openssl conf")
	}

	outCertPemPath := filepath.Join(dir, "rootca.pem")

	// new root ca
	if _, err = t.runCMD(ctx, []string{
		"req", "-outform", "PEM", "-out", outCertPemPath,
		"-key", "/dev/stdin",
		"-set_serial", strconv.Itoa(int(t.serialGenerator.SerialNum())),
		"-days", strconv.Itoa(1 + int(time.Until(opt.notAfter)/time.Hour/24)),
		"-x509", "-new", "-nodes", "-utf8", "-batch",
		"-sm3",
		"-copy_extensions", "copyall",
		"-extensions", "v3_ca",
		"-config", confPath,
	}, prikeyPem); err != nil {
		return nil, errors.Wrap(err, "generate new root ca")
	}

	certPem, err := os.ReadFile(outCertPemPath)
	if err != nil {
		return nil, errors.Wrap(err, "read root ca")
	}

	if certDer, err = Pem2Der(certPem); err != nil {
		return nil, errors.Wrap(err, "Pem2Der")
	}

	return certDer, nil
}

// NewX509CSR generate new x509 csr
func (t *Tongsuo) NewX509CSR(ctx context.Context, prikeyPem []byte, opts ...X509CSROption) (csrDer []byte, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
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
		"-sm3",
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

	digestAlgo := "-sha256"
	if certinfo, err := t.ShowCertInfo(ctx, parentCertDer); err != nil {
		return nil, errors.Wrap(err, "show parent cert info")
	} else if strings.Contains(string(certinfo.Raw), "ASN1 OID: SM2") {
		digestAlgo = "-sm3"
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	confPath := filepath.Join(dir, "csr.cnf")
	if err = os.WriteFile(confPath, opensslConf, 0600); err != nil {
		return nil, errors.Wrap(err, "write openssl conf")
	}

	// fmt.Println(string(opensslConf)) // FIXME

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
		digestAlgo,
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

// EncryptBySm4CbcBaisc encrypt by sm4
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

	if hmac, err = HMACSha256(key, ciphertext); err != nil {
		return nil, nil, errors.Wrap(err, "calculate hmac")
	}

	return ciphertext, hmac, nil
}

// DecryptBySm4CbcBaisc decrypt by sm4
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
		if expectedHmac, err := HMACSha256(key, ciphertext); err != nil {
			return nil, errors.Wrap(err, "calculate hmac")
		} else if !bytes.Equal(hmac, expectedHmac) {
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
// the first element of certsPem should be leaf cert, the last element should be root ca,
// and the intermediate certs should be in the middle if exists.
func (t *Tongsuo) VerifyCertsChain(ctx context.Context, certsPem [][]byte) error {
	if len(certsPem) < 2 {
		return errors.Errorf("certs chain should have at least 2 certs")
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	// write leaf cert
	leafCertPath := filepath.Join(dir, "leaf.crt")
	if err = os.WriteFile(leafCertPath, certsPem[0], 0600); err != nil {
		return errors.Wrap(err, "write leaf cert")
	}

	// write root ca
	rootCaPath := filepath.Join(dir, "rootca.crt")
	if err = os.WriteFile(rootCaPath, certsPem[len(certsPem)-1], 0600); err != nil {
		return errors.Wrap(err, "write root ca")
	}

	// write intermediate certs
	var interCaPath string
	if len(certsPem) > 2 {
		interCaPems := bytes.Join(certsPem[1:len(certsPem)-1], nil)
		interCaPath := filepath.Join(dir, "interca.crt")
		if err := os.WriteFile(interCaPath, interCaPems, 0600); err != nil {
			return errors.Wrap(err, "write intermediate certs")
		}
	}

	cmd := []string{
		"verify", "-CAfile", rootCaPath,
	}
	if len(certsPem) > 2 {
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
