package crypto

import (
	"context"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"

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
	// validity, when set, bounds the issued NotBefore/NotAfter.
	validity *tongsuoValidity
	// names, when set, is the exact subject and SAN content required.
	names *tongsuoNames
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

	if want.validity != nil {
		if err = want.validity.verify(cert); err != nil {
			return errors.Wrap(err, "verify issued validity")
		}
	}

	if want.names != nil {
		if err = want.names.verify(tongsuoNames{
			subject:  cert.Subject,
			dnsNames: cert.DNSNames,
			emails:   cert.EmailAddresses,
			ips:      cert.IPAddresses,
			uris:     cert.URIs,
		}); err != nil {
			return errors.Wrap(err, "verify issued names")
		}
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
// request a non-CA certificate carries exactly anyExtendedKeyUsage.
//
// The validity window is validated against one captured clock value before
// issuance: zero, past (NotAfter <= now), empty and inverted windows are
// rejected. Binaries supporting -not_before/-not_after receive the exact UTC
// bounds (truncated to whole seconds); older binaries receive a conservative
// whole-day -days value and requests that cannot be represented without
// extending NotAfter are rejected. The issued certificate is parsed and its
// validity and EKUs are verified before any bytes are returned.
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

	opensslConf, err := x509Cert2OpensslConf(tpl, opensslConfEncoder{})
	if err != nil {
		return nil, errors.Wrap(err, "marshal openssl conf")
	}
	extKeyUsages, err := tongsuoCertExtKeyUsages(tpl)
	if err != nil {
		return nil, errors.Wrap(err, "ext key usage")
	}
	validity, validityArgs, err := t.validityArgs(ctx, opt.notBefore, opt.notAfter)
	if err != nil {
		return nil, errors.Wrap(err, "validity")
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
	args := slices.Concat([]string{
		tongsuoCmdReq, tongsuoFlagOutform, "PEM", tongsuoFlagOut, outCertPemPath,
		"-key", tongsuoStdinPath,
		"-set_serial", tpl.SerialNumber.String(),
	}, validityArgs, []string{
		"-x509", "-new", "-nodes", "-utf8", "-batch",
		tongsuoDigestSM3,
		"-copy_extensions", "copyall",
		"-extensions", "v3_ca",
		tongsuoFlagConfig, confPath,
	})
	if _, err = t.runCMD(ctx, args, prikeyPem); err != nil {
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
		validity:          &validity,
		names: &tongsuoNames{
			subject:  tpl.Subject,
			dnsNames: tpl.DNSNames,
			emails:   tpl.EmailAddresses,
			ips:      tpl.IPAddresses,
			uris:     tpl.URIs,
		},
	}); err != nil {
		return nil, errors.Wrap(err, "verify issued certificate")
	}

	return certDer, nil
}

// NewX509CSR generates a CSR for prikeyPem through the tongsuo binary and
// returns its DER.
//
// Subject and SAN values are written to the configuration as literal values,
// so OpenSSL variable expansion, quoting, escapes and comments never apply,
// and values that cannot be encoded exactly (control characters, non-ASCII
// SANs, the email keywords "copy"/"move") are rejected. The generated CSR is
// parsed, its signature verified and its subject and SANs compared with the
// request before it is returned.
func (t *Tongsuo) NewX509CSR(ctx context.Context, prikeyPem []byte, opts ...X509CSROption) (csrDer []byte, err error) {
	tpl, err := X509CsrOption2Template(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "X509CsrOption2Template")
	}

	opensslConf, err := x509Csr2OpensslConf(tpl, opensslConfEncoder{})
	if err != nil {
		return nil, errors.Wrap(err, "marshal openssl conf")
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

	outCsrDerPath := filepath.Join(dir, "csr.der")

	if _, err = t.runCMD(ctx, []string{
		tongsuoCmdReq, "-new", tongsuoFlagOutform, tongsuoFormatDER, tongsuoFlagOut, outCsrDerPath,
		"-key", tongsuoStdinPath,
		"-utf8",
		tongsuoDigestSM3,
		tongsuoFlagConfig, confPath,
	}, prikeyPem); err != nil {
		return nil, errors.Wrap(err, "generate new csr")
	}

	if csrDer, err = os.ReadFile(outCsrDerPath); err != nil {
		return nil, errors.Wrap(err, "read csr")
	}

	csr, err := smx509.ParseCertificateRequest(csrDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse generated csr")
	}
	if err = csr.CheckSignature(); err != nil {
		return nil, errors.Wrap(err, "verify generated csr signature")
	}
	if err = (tongsuoNames{
		subject:  tpl.Subject,
		dnsNames: tpl.DNSNames,
		emails:   tpl.EmailAddresses,
		ips:      tpl.IPAddresses,
		uris:     tpl.URIs,
	}).verify(tongsuoNames{
		subject:  csr.Subject,
		dnsNames: csr.DNSNames,
		emails:   csr.EmailAddresses,
		ips:      csr.IPAddresses,
		uris:     csr.URIs,
	}); err != nil {
		return nil, errors.Wrap(err, "verify generated csr names")
	}

	return csrDer, nil
}

// signX509CSR implements Tongsuo.NewX509CertByCSR: it validates the requested
// validity window, signs csrDer with the parent certificate and key through
// `tongsuo x509 -req`, then verifies the issued certificate before returning
// its DER. The issued validity must lie within the request and explicitly
// requested extended key usages must appear exactly; otherwise an error is
// returned.
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
	validity, validityArgs, err := t.validityArgs(ctx, opt.notBefore, opt.notAfter)
	if err != nil {
		return nil, errors.Wrap(err, "validity")
	}

	// verify request integrity and proof of possession before invoking the
	// tool, independently of the binary's own check
	csr, err := smx509.ParseCertificateRequest(csrDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse csr")
	}
	if err = csr.CheckSignature(); err != nil {
		return nil, errors.Wrap(err, "verify csr signature")
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

	args := slices.Concat([]string{
		tongsuoCmdX509, "-req", tongsuoFlagOutform, tongsuoFormatDER, tongsuoFlagOut, outCertDerPath,
		tongsuoFlagIn, csrDerPath, tongsuoFlagInform, tongsuoFormatDER,
		"-CA", parentCertDerPath, "-CAkey", tongsuoStdinPath, "-CAcreateserial",
	}, validityArgs, []string{
		digestAlgo,
		"-copy_extensions", "copyall",
		"-extfile", confPath, "-extensions", "v3_ca",
	})
	if _, err = t.runCMD(ctx, args, parentPrikeyPem); err != nil {
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
		validity:          &validity,
	}); err != nil {
		return nil, errors.Wrap(err, "verify issued certificate")
	}

	return certDer, nil
}
