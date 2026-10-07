package crypto

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/emmansun/gmsm/sm2"
	"github.com/emmansun/gmsm/smx509"

	glog "github.com/Laisky/go-utils/v6/log"
)

// maxTongsuoCertInfoDERSize bounds the certificate DER accepted by the
// structured certificate parser and forwarded to the display subprocess.
const maxTongsuoCertInfoDERSize = 1 << 20

// ErrTongsuoUnsupportedPublicKeyAlgorithm is returned (wrapped) when a
// certificate's subject public key algorithm is not one of RSA, ECDSA,
// Ed25519 or SM2. Callers can detect it with errors.Is.
var ErrTongsuoUnsupportedPublicKeyAlgorithm = errors.New("unsupported certificate public key algorithm")

// OpensslCertificateOutput output of `openssl x509 -inform DER -text`
//
// Deprecated: ShowCertInfo no longer parses display text; use
// ParseTongsuoCertInfo or Tongsuo.ShowCertInfoDetail for structured metadata.
type OpensslCertificateOutput struct {
	// Raw is the raw output of `openssl x509 -inform DER -text`
	Raw                                          []byte
	SerialNumber                                 *big.Int
	NotBefore, NotAfter                          time.Time
	IsCa                                         bool
	Subject                                      pkix.Name
	Policies                                     []asn1.ObjectIdentifier
	PublicKeyAlgorithm                           x509.PublicKeyAlgorithm
	SubjectKeyIdentifier, AuthorityKeyIdentifier []byte
}

// TongsuoPublicKeyAlgorithm names the subject public key algorithm of a
// certificate parsed by ParseTongsuoCertInfo. Unlike x509.PublicKeyAlgorithm
// it distinguishes SM2 from ECDSA.
type TongsuoPublicKeyAlgorithm string

const (
	// TongsuoPublicKeyAlgorithmRSA is an RSA subject public key.
	TongsuoPublicKeyAlgorithmRSA TongsuoPublicKeyAlgorithm = "RSA"
	// TongsuoPublicKeyAlgorithmECDSA is an ECDSA key on a curve supported by crypto/x509.
	TongsuoPublicKeyAlgorithmECDSA TongsuoPublicKeyAlgorithm = "ECDSA"
	// TongsuoPublicKeyAlgorithmEd25519 is an Ed25519 subject public key.
	TongsuoPublicKeyAlgorithmEd25519 TongsuoPublicKeyAlgorithm = "Ed25519"
	// TongsuoPublicKeyAlgorithmSM2 is an SM2 key (id-ecPublicKey on the SM2 curve).
	TongsuoPublicKeyAlgorithmSM2 TongsuoPublicKeyAlgorithm = "SM2"
)

// TongsuoCertInfo is certificate metadata parsed structurally from DER.
//
// Every security-relevant field (subject, issuer, CA status, key usages,
// policies, SANs, validity, serial number and key algorithm) comes from the
// ASN.1 structure, never from display text.
type TongsuoCertInfo struct {
	// Display is the human-readable `tongsuo x509 -text` rendering. It is
	// informational only: it is never parsed and must not be used for any
	// security decision. ParseTongsuoCertInfo leaves it empty.
	Display string
	// Certificate is the parsed certificate. For RSA, ECDSA and Ed25519 keys it
	// is exactly the result of x509.ParseCertificate. For SM2 keys, which
	// crypto/x509 cannot represent, every field is copied from the SM2-capable
	// parser except that PublicKeyAlgorithm is x509.UnknownPublicKeyAlgorithm,
	// PublicKey is nil and an SM2-with-SM3 signature is reported as
	// x509.UnknownSignatureAlgorithm, so SM2 is never mislabeled as ECDSA.
	Certificate *x509.Certificate
	// PublicKeyAlgorithm is the subject public key algorithm, distinguishing SM2.
	PublicKeyAlgorithm TongsuoPublicKeyAlgorithm
	// PublicKey is the parsed subject public key. For SM2 it is an
	// *ecdsa.PublicKey on the gmsm sm2.P256() curve that must only be used with
	// SM2 algorithms, never with crypto/ecdsa.
	PublicKey crypto.PublicKey
	// SignatureAlgorithm is the certificate signature algorithm name, such as
	// "SHA256-RSA" or "SM2-SM3".
	SignatureAlgorithm string
}

// IsSM2 reports whether the certificate's subject public key is an SM2 key.
// It returns false for a nil receiver.
func (i *TongsuoCertInfo) IsSM2() bool {
	return i != nil && i.PublicKeyAlgorithm == TongsuoPublicKeyAlgorithmSM2
}

// isSM2PublicKey reports whether pub is an SM2 public key as returned by the
// smx509 parser.
func isSM2PublicKey(pub any) bool {
	key, ok := pub.(*ecdsa.PublicKey)
	return ok && key.Curve == sm2.P256()
}

// ParseTongsuoCertInfo parses exactly one DER certificate without running any
// subprocess. RSA, ECDSA and Ed25519 certificates are parsed by
// x509.ParseCertificate; SM2 certificates by the SM2-capable smx509 parser.
// It returns an error wrapping ErrTongsuoUnsupportedPublicKeyAlgorithm for any
// other public key algorithm, and a parse error for malformed DER or trailing
// data. The returned Display field is empty.
func ParseTongsuoCertInfo(certDer []byte) (*TongsuoCertInfo, error) {
	if len(certDer) == 0 {
		return nil, errors.New("certificate DER is empty")
	}
	if len(certDer) > maxTongsuoCertInfoDERSize {
		return nil, errors.Errorf("certificate DER exceeds %d bytes", maxTongsuoCertInfoDERSize)
	}

	cert, nativeErr := x509.ParseCertificate(certDer)
	if nativeErr == nil {
		// crypto/x509 cannot parse SM2 keys, so a native success is never SM2.
		var algo TongsuoPublicKeyAlgorithm
		switch cert.PublicKeyAlgorithm {
		case x509.RSA:
			algo = TongsuoPublicKeyAlgorithmRSA
		case x509.ECDSA:
			algo = TongsuoPublicKeyAlgorithmECDSA
		case x509.Ed25519:
			algo = TongsuoPublicKeyAlgorithmEd25519
		default:
			return nil, errors.Wrapf(ErrTongsuoUnsupportedPublicKeyAlgorithm,
				"public key algorithm %q", cert.PublicKeyAlgorithm.String())
		}

		sigName := cert.SignatureAlgorithm.String()
		if cert.SignatureAlgorithm == x509.UnknownSignatureAlgorithm {
			// e.g. an RSA leaf signed by an SM2 issuer
			if smCert, err := smx509.ParseCertificate(certDer); err == nil {
				sigName = smCert.SignatureAlgorithm.String()
			}
		}

		return &TongsuoCertInfo{
			Certificate:        cert,
			PublicKeyAlgorithm: algo,
			PublicKey:          cert.PublicKey,
			SignatureAlgorithm: sigName,
		}, nil
	}

	smCert, smErr := smx509.ParseCertificate(certDer)
	if smErr != nil {
		return nil, errors.Wrap(nativeErr, "parse certificate DER")
	}
	if !isSM2PublicKey(smCert.PublicKey) {
		return nil, errors.Wrapf(ErrTongsuoUnsupportedPublicKeyAlgorithm,
			"public key algorithm %q", smCert.PublicKeyAlgorithm.String())
	}

	view, err := smx509CertificateToX509(smCert)
	if err != nil {
		return nil, errors.Wrap(err, "convert SM2 certificate")
	}

	return &TongsuoCertInfo{
		Certificate:        view,
		PublicKeyAlgorithm: TongsuoPublicKeyAlgorithmSM2,
		PublicKey:          smCert.PublicKey,
		SignatureAlgorithm: smCert.SignatureAlgorithm.String(),
	}, nil
}

// ShowCertInfoDetail parses certDer structurally with ParseTongsuoCertInfo and
// attaches the human-readable `tongsuo x509 -text` rendering as Display. The
// DER is validated before any subprocess runs. It returns the parsed metadata
// or an error; unsupported public key algorithms fail closed with an error
// wrapping ErrTongsuoUnsupportedPublicKeyAlgorithm.
func (t *Tongsuo) ShowCertInfoDetail(ctx context.Context, certDer []byte) (*TongsuoCertInfo, error) {
	info, err := ParseTongsuoCertInfo(certDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse certificate")
	}

	output, err := t.runCMD(ctx, []string{
		tongsuoCmdX509, tongsuoFlagInform, tongsuoFormatDER, tongsuoFlagText,
	}, certDer)
	if err != nil {
		return nil, errors.Wrap(err, "run cmd to show cert info")
	}

	info.Display = string(output)
	glog.Shared.Debug("parsed tongsuo certificate info",
		zap.String("public_key_algorithm", string(info.PublicKeyAlgorithm)),
		zap.String("signature_algorithm", info.SignatureAlgorithm),
		zap.Bool("is_ca", info.Certificate.IsCA))
	return info, nil
}

// ShowCertInfo returns the human-readable `tongsuo x509 -text` rendering of
// certDer and the certificate parsed structurally from DER.
//
// The returned certinfo string is display text only and is never parsed. The
// returned certificate follows the TongsuoCertInfo.Certificate contract: for
// SM2 keys PublicKeyAlgorithm is x509.UnknownPublicKeyAlgorithm and PublicKey is
// nil; use ShowCertInfoDetail to identify SM2 explicitly. Unsupported public key
// algorithms fail closed with an error wrapping
// ErrTongsuoUnsupportedPublicKeyAlgorithm.
func (t *Tongsuo) ShowCertInfo(ctx context.Context,
	certDer []byte) (
	certinfo string, cert *x509.Certificate, err error) {
	info, err := t.ShowCertInfoDetail(ctx, certDer)
	if err != nil {
		return "", nil, errors.Wrap(err, "show cert info")
	}

	return info.Display, info.Certificate, nil
}

// ShowCsrInfo returns the human-readable `tongsuo req -text` rendering of
// csrDer, for display only.
//
// `req` reads an OpenSSL configuration file even when only printing; an
// explicit empty configuration is passed so the output does not depend on the
// system default file or on OPENSSL_CONF, which subprocesses do not inherit.
// It returns the rendering or an error.
func (t *Tongsuo) ShowCsrInfo(ctx context.Context, csrDer []byte) (
	output string, err error) {
	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return "", errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	confPath := filepath.Join(dir, "empty.cnf")
	if err = os.WriteFile(confPath, nil, 0600); err != nil {
		return "", errors.Wrap(err, "write empty openssl conf")
	}

	out, err := t.runCMD(ctx, []string{
		tongsuoCmdReq, tongsuoFlagInform, tongsuoFormatDER, tongsuoFlagText,
		tongsuoFlagConfig, confPath,
	}, csrDer)
	if err != nil {
		return "", errors.Wrap(err, "run cmd to show csr info")
	}

	return string(out), nil
}
