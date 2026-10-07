package crypto

import (
	"context"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/emmansun/gmsm/smx509"
)

// tongsuoIssuanceExpectation describes the properties a certificate produced
// by the tongsuo binary must have before its bytes are returned to callers.
type tongsuoIssuanceExpectation struct {
	// checkExtKeyUsages enables the exact extended key usage comparison.
	checkExtKeyUsages bool
	// extKeyUsages is the exact, canonical extended key usage list expected
	// when checkExtKeyUsages is set; nil means no EKU extension.
	extKeyUsages []x509.ExtKeyUsage
}

// verifyTongsuoIssuedCert parses certDer with the SM2-capable parser and checks
// it against want. It returns nil only when certDer is exactly one certificate
// that satisfies every enabled expectation, so tool or version differences fail
// closed instead of returning an unexpected certificate.
func verifyTongsuoIssuedCert(certDer []byte, want tongsuoIssuanceExpectation) error {
	cert, err := smx509.ParseCertificate(certDer)
	if err != nil {
		return errors.Wrap(err, "parse issued certificate")
	}

	if want.checkExtKeyUsages {
		if len(cert.UnknownExtKeyUsage) != 0 {
			return errors.Errorf("issued certificate carries unrequested ext key usages %v",
				cert.UnknownExtKeyUsage)
		}

		got, err := smx509ExtKeyUsagesToX509(cert.ExtKeyUsage)
		if err != nil {
			return errors.Wrap(err, "convert issued ext key usages")
		}
		if !slices.Equal(got, want.extKeyUsages) {
			return errors.Errorf("issued certificate ext key usages %v differ from requested %v",
				got, want.extKeyUsages)
		}
	}

	return nil
}

// NewX509Cert generates a self-signed x509 certificate for prikeyPem through
// the tongsuo binary and returns its DER.
//
// Extended key usages are emitted exactly as requested; without an explicit
// request a non-CA certificate carries exactly anyExtendedKeyUsage. The issued
// certificate is parsed and verified before it is returned.
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

	opensslConf, err := x509Cert2OpensslConf(tpl)
	if err != nil {
		return nil, errors.Wrap(err, "marshal openssl conf")
	}
	extKeyUsages, err := tongsuoCertExtKeyUsages(tpl)
	if err != nil {
		return nil, errors.Wrap(err, "ext key usage")
	}

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
		tongsuoCmdReq, tongsuoFlagOutform, "PEM", tongsuoFlagOut, outCertPemPath,
		"-key", tongsuoStdinPath,
		"-set_serial", tpl.SerialNumber.String(),
		"-days", strconv.Itoa(1 + int(time.Until(opt.notAfter)/time.Hour/24)),
		"-x509", "-new", "-nodes", "-utf8", "-batch",
		tongsuoDigestSM3,
		"-copy_extensions", "copyall",
		"-extensions", "v3_ca",
		tongsuoFlagConfig, confPath,
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

	if err = verifyTongsuoIssuedCert(certDer, tongsuoIssuanceExpectation{
		checkExtKeyUsages: true,
		extKeyUsages:      extKeyUsages,
	}); err != nil {
		return nil, errors.Wrap(err, "verify issued certificate")
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
		tongsuoCmdReq, "-new", tongsuoFlagOutform, tongsuoFormatDER, tongsuoFlagOut, outCsrDerPath,
		"-key", tongsuoStdinPath,
		tongsuoDigestSM3,
		tongsuoFlagConfig, confPath,
	}, prikeyPem); err != nil {
		return nil, errors.Wrap(err, "generate new csr")
	}

	if csrDer, err = os.ReadFile(outCsrDerPath); err != nil {
		return nil, errors.Wrap(err, "read csr")
	}

	return csrDer, nil
}

// signX509CSR implements Tongsuo.NewX509CertByCSR: it signs csrDer with the
// parent certificate and key through `tongsuo x509 -req`, then verifies the
// issued certificate before returning its DER. Explicitly requested extended
// key usages must appear exactly; otherwise an error is returned.
func (t *Tongsuo) signX509CSR(ctx context.Context,
	parentCertDer []byte,
	parentPrikeyPem []byte,
	csrDer []byte,
	opts ...SignCSROption) (certDer []byte, err error) {
	opt, opensslConf, err := x509SignCsrOptions2OpensslConf(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "X509SignCsrOptions2OpensslConf")
	}
	extKeyUsages, err := canonicalExtKeyUsages(opt.extKeyUsage)
	if err != nil {
		return nil, errors.Wrap(err, "ext key usage")
	}

	// select the digest from the parsed parent key, never from display text
	digestAlgo := tongsuoDigestSHA256
	if parentInfo, err := ParseTongsuoCertInfo(parentCertDer); err != nil {
		return nil, errors.Wrap(err, "parse parent cert")
	} else if parentInfo.IsSM2() {
		digestAlgo = tongsuoDigestSM3
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
		tongsuoCmdX509, "-req", tongsuoFlagOutform, tongsuoFormatDER, tongsuoFlagOut, outCertDerPath,
		tongsuoFlagIn, csrDerPath, tongsuoFlagInform, tongsuoFormatDER,
		"-CA", parentCertDerPath, "-CAkey", tongsuoStdinPath, "-CAcreateserial",
		"-days", strconv.Itoa(int(time.Until(opt.notAfter) / time.Hour / 24)),
		digestAlgo,
		"-copy_extensions", "copyall",
		"-extfile", confPath, "-extensions", "v3_ca",
	}, parentPrikeyPem); err != nil {
		return nil, errors.Wrap(err, "run tongsuo x509 -req")
	}

	if certDer, err = os.ReadFile(outCertDerPath); err != nil {
		return nil, errors.Wrap(err, "read signed cert")
	}

	// the CSR may carry its own EKU (copied by copy_extensions); only an
	// explicit request is enforced here
	if err = verifyTongsuoIssuedCert(certDer, tongsuoIssuanceExpectation{
		checkExtKeyUsages: len(extKeyUsages) != 0,
		extKeyUsages:      extKeyUsages,
	}); err != nil {
		return nil, errors.Wrap(err, "verify issued certificate")
	}

	return certDer, nil
}
