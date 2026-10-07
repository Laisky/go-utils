package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"os/exec"
	"testing"
	"time"

	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
)

// newSecurityTestTongsuo returns a Tongsuo wrapper backed by the real tongsuo
// executable found on PATH. It skips the calling test when tongsuo is not
// installed, and fails the test when the wrapper cannot be constructed.
func newSecurityTestTongsuo(t *testing.T) *Tongsuo {
	t.Helper()

	exePath, err := exec.LookPath("tongsuo")
	if err != nil {
		t.Skip("tongsuo executable is not installed")
	}

	ins, err := NewTongsuo(exePath)
	require.NoError(t, err)
	return ins
}

// parseSMCertForTest parses certDer with the SM2-capable smx509 parser and
// fails the test when the DER is not exactly one certificate.
func parseSMCertForTest(t *testing.T, certDer []byte) *smx509.Certificate {
	t.Helper()

	cert, err := smx509.ParseCertificate(certDer)
	require.NoError(t, err)
	return cert
}

// newTongsuoSM2CAForTest creates an SM2 self-signed CA with the real tongsuo
// binary. It returns the CA private key PEM and certificate DER.
func newTongsuoSM2CAForTest(t *testing.T, ins *Tongsuo, commonName string) (prikeyPem, certDer []byte) {
	t.Helper()

	prikeyPem, certDer, err := ins.NewPrikeyAndCert(t.Context(),
		WithX509CertCommonName(commonName),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)
	return prikeyPem, certDer
}

// newTongsuoSM2CSRForTest creates a fresh SM2 key and CSR with the real tongsuo
// binary. It returns the CSR DER.
func newTongsuoSM2CSRForTest(t *testing.T, ins *Tongsuo, opts ...X509CSROption) []byte {
	t.Helper()

	prikeyPem, err := ins.NewPrikey(t.Context())
	require.NoError(t, err)

	csrDer, err := ins.NewX509CSR(t.Context(), prikeyPem, opts...)
	require.NoError(t, err)
	return csrDer
}

// newGoFixtureCertForTest signs tpl with a fresh ECDSA P-256 issuer key and
// returns the certificate DER. pub is the subject public key; when it is nil the
// issuer key is used, producing a self-signed certificate.
func newGoFixtureCertForTest(t *testing.T, tpl *x509.Certificate, pub crypto.PublicKey) []byte {
	t.Helper()

	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	if pub == nil {
		pub = issuerKey.Public()
	}
	if tpl.SerialNumber == nil {
		tpl.SerialNumber = big.NewInt(4242)
	}
	if tpl.NotBefore.IsZero() {
		tpl.NotBefore = time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	}
	if tpl.NotAfter.IsZero() {
		tpl.NotAfter = tpl.NotBefore.Add(48 * time.Hour)
	}
	if len(tpl.SubjectKeyId) == 0 {
		tpl.SubjectKeyId = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, issuerKey)
	require.NoError(t, err)
	return der
}

// handBuiltTBSCertificateForTest mirrors the RFC 5280 TBSCertificate structure
// so tests can embed subject public keys that crypto/x509 refuses to encode.
type handBuiltTBSCertificateForTest struct {
	Version            int `asn1:"optional,explicit,default:0,tag:0"`
	SerialNumber       *big.Int
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Issuer             asn1.RawValue
	Validity           struct{ NotBefore, NotAfter time.Time }
	Subject            asn1.RawValue
	PublicKey          asn1.RawValue
	Extensions         []pkix.Extension `asn1:"optional,explicit,tag:3"`
}

// handBuiltCertificateForTest mirrors the RFC 5280 Certificate structure.
type handBuiltCertificateForTest struct {
	TBSCertificate     asn1.RawValue
	SignatureAlgorithm pkix.AlgorithmIdentifier
	SignatureValue     asn1.BitString
}

// newHandBuiltCertForTest builds a v3 certificate whose SubjectPublicKeyInfo
// is the given DER and whose subject and issuer are subject, signed with a
// throwaway ECDSA P-256 key and carrying a subject key identifier extension.
// It returns the certificate DER. It exists to produce public key algorithms
// that the Go certificate builder rejects, such as X25519.
func newHandBuiltCertForTest(t *testing.T, spkiDer []byte, subject pkix.Name) []byte {
	t.Helper()

	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	name, err := asn1.Marshal(subject.ToRDNSequence())
	require.NoError(t, err)
	ski, err := asn1.Marshal([]byte{0x0a, 0x0b, 0x0c, 0x0d})
	require.NoError(t, err)

	sigAlg := pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}}
	now := time.Now().UTC().Truncate(time.Second)
	tbs := handBuiltTBSCertificateForTest{
		Version:            2,
		SerialNumber:       big.NewInt(77),
		SignatureAlgorithm: sigAlg,
		Issuer:             asn1.RawValue{FullBytes: name},
		Subject:            asn1.RawValue{FullBytes: name},
		PublicKey:          asn1.RawValue{FullBytes: spkiDer},
		Extensions: []pkix.Extension{{
			Id:    asn1.ObjectIdentifier{2, 5, 29, 14},
			Value: ski,
		}},
	}
	tbs.Validity.NotBefore = now.Add(-time.Hour)
	tbs.Validity.NotAfter = now.Add(time.Hour)

	tbsDer, err := asn1.Marshal(tbs)
	require.NoError(t, err)

	digest := sha256.Sum256(tbsDer)
	sig, err := ecdsa.SignASN1(rand.Reader, signer, digest[:])
	require.NoError(t, err)

	der, err := asn1.Marshal(handBuiltCertificateForTest{
		TBSCertificate:     asn1.RawValue{FullBytes: tbsDer},
		SignatureAlgorithm: sigAlg,
		SignatureValue:     asn1.BitString{Bytes: sig, BitLength: 8 * len(sig)},
	})
	require.NoError(t, err)
	return der
}

// newGoRevocationFixtureForTest builds an unsigned-intent CRL template signed
// by a throwaway ECDSA key, with issuer name and authority key ID copied from
// the given issuer fields. It returns the CRL DER that SignX509CRL re-signs.
func newGoRevocationFixtureForTest(t *testing.T, issuerRawSubject, issuerSKI []byte,
	number *big.Int, entries []x509.RevocationListEntry, thisUpdate, nextUpdate time.Time) []byte {
	t.Helper()

	throwaway, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	issuer := &x509.Certificate{
		RawSubject:   issuerRawSubject,
		SubjectKeyId: issuerSKI,
		KeyUsage:     x509.KeyUsageCRLSign,
		Subject:      pkix.Name{CommonName: "placeholder"},
	}
	crlDer, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    number,
		ThisUpdate:                thisUpdate,
		NextUpdate:                nextUpdate,
		RevokedCertificateEntries: entries,
	}, issuer, throwaway)
	require.NoError(t, err)
	return crlDer
}
