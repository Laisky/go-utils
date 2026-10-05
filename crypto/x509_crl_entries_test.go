package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// security66Issuer creates a disposable standard-library CA and key for real CRL signing tests.
func security66Issuer(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "local CRL test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SubjectKeyId: []byte{1, 2, 3, 4}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return ca, key
}

// TestSecurity66CRLEntryExtensionsSurvive verifies encoded DER, signature, reason, and opaque extension preservation.
func TestSecurity66CRLEntryExtensionsSurvive(t *testing.T) {
	ca, key := security66Issuer(t)
	now := time.Now().UTC().Truncate(time.Second)
	reason, err := asn1.Marshal(asn1.Enumerated(1))
	require.NoError(t, err)
	invalidity, err := asn1.MarshalWithParams(now.Add(-time.Hour), "generalized")
	require.NoError(t, err)
	input := pkix.RevokedCertificate{SerialNumber: big.NewInt(42), RevocationTime: now,
		Extensions: []pkix.Extension{
			{Id: asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6}, Critical: true, Value: []byte{5, 0}},
			{Id: asn1.ObjectIdentifier{2, 5, 29, 24}, Value: invalidity},
			{Id: asn1.ObjectIdentifier{2, 5, 29, 21}, Value: reason},
		}}
	der, err := NewX509CRL(ca, key, big.NewInt(7), []pkix.RevokedCertificate{input})
	require.NoError(t, err)
	crl, err := x509.ParseRevocationList(der)
	require.NoError(t, err)
	require.NoError(t, crl.CheckSignatureFrom(ca))
	require.Equal(t, big.NewInt(7), crl.Number)
	require.Len(t, crl.RevokedCertificateEntries, 1)
	entry := crl.RevokedCertificateEntries[0]
	require.Equal(t, input.SerialNumber, entry.SerialNumber)
	require.True(t, input.RevocationTime.Equal(entry.RevocationTime))
	require.Equal(t, 1, entry.ReasonCode)
	require.ElementsMatch(t, input.Extensions, entry.Extensions)
}

// TestSecurity66InvalidEntryExtensionsFailClosed rejects ambiguous reasons and unsupported indirect-CRL semantics without partial output.
func TestSecurity66InvalidEntryExtensionsFailClosed(t *testing.T) {
	ca, key := security66Issuer(t)
	reasonOID := asn1.ObjectIdentifier{2, 5, 29, 21}
	reason := pkix.Extension{Id: reasonOID, Value: []byte{10, 1, 1}}
	for name, extensions := range map[string][]pkix.Extension{
		"duplicate reason":   {reason, reason},
		"conflicting reason": {reason, {Id: reasonOID, Value: []byte{10, 1, 2}}},
		"wrong ASN.1 type":   {{Id: reasonOID, Value: []byte{2, 1, 1}}},
		"trailing ASN.1":     {{Id: reasonOID, Value: []byte{10, 1, 1, 5, 0}}},
		"negative reason":    {{Id: reasonOID, Value: []byte{10, 1, 255}}},
		"reserved reason":    {{Id: reasonOID, Value: []byte{10, 1, 7}}},
		"unknown reason":     {{Id: reasonOID, Value: []byte{10, 1, 11}}},
		"critical reason":    {{Id: reasonOID, Critical: true, Value: []byte{10, 1, 1}}},
		"indirect issuer":    {{Id: asn1.ObjectIdentifier{2, 5, 29, 29}, Critical: true, Value: []byte{48, 0}}},
		"duplicate custom":   {{Id: asn1.ObjectIdentifier{1, 2, 3}, Value: []byte{5, 0}}, {Id: asn1.ObjectIdentifier{1, 2, 3}, Value: []byte{5, 0}}},
	} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509CRL(ca, key, big.NewInt(8), []pkix.RevokedCertificate{{SerialNumber: big.NewInt(42), RevocationTime: time.Now().UTC(), Extensions: extensions}})
			require.Error(t, err)
			require.Nil(t, der)
		})
	}
}

// TestSecurity66ValidReasonsRoundTrip exercises the complete RFC reason enumeration, including omission of unspecified zero.
func TestSecurity66ValidReasonsRoundTrip(t *testing.T) {
	ca, key := security66Issuer(t)
	for _, code := range []int{0, 1, 2, 3, 4, 5, 6, 8, 9, 10} {
		value, err := asn1.Marshal(asn1.Enumerated(code))
		require.NoError(t, err)
		input := pkix.RevokedCertificate{SerialNumber: big.NewInt(42), RevocationTime: time.Now().UTC(),
			Extensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 21}, Value: value}}}
		der, err := NewX509CRL(ca, key, big.NewInt(9), []pkix.RevokedCertificate{input})
		require.NoError(t, err)
		crl, err := x509.ParseRevocationList(der)
		require.NoError(t, err)
		require.Equal(t, code, crl.RevokedCertificateEntries[0].ReasonCode)
		require.NoError(t, crl.CheckSignatureFrom(ca))
		if code == 0 {
			require.Empty(t, crl.RevokedCertificateEntries[0].Extensions)
		}
	}
}
