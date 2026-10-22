package crypto

import (
	"crypto"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// requirePathLenDER checks both the parsed constraint and its actual ASN.1 presence.
func requirePathLenDER(t *testing.T, cert *x509.Certificate, maximum int) {
	t.Helper()
	require.True(t, cert.IsCA)
	require.True(t, cert.BasicConstraintsValid)
	require.Equal(t, maximum, cert.MaxPathLen)
	require.Equal(t, maximum == 0, cert.MaxPathLenZero)
	var count int
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 19}) {
			continue
		}
		count++
		require.True(t, ext.Critical)
		var fields []asn1.RawValue
		rest, err := asn1.Unmarshal(ext.Value, &fields)
		require.NoError(t, err)
		require.Empty(t, rest)
		if maximum < 0 {
			require.Len(t, fields, 1, "an unset constraint must omit the INTEGER")
		} else {
			require.Len(t, fields, 2)
			require.Equal(t, asn1.ClassUniversal, fields[1].Class)
			require.Equal(t, asn1.TagInteger, fields[1].Tag)
			encoded, err := asn1.Marshal(maximum)
			require.NoError(t, err)
			require.Equal(t, encoded, fields[1].FullBytes)
		}
		require.Equal(t, []byte{1, 1, 0xff}, fields[0].FullBytes)
	}
	require.Equal(t, 1, count, "exactly one BasicConstraints extension")
}

// TestX509PathLenDER preserves absent, explicit-zero and positive constraints on both issuance paths.
func TestX509PathLenDER(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	issuer := newCSRSignatureTestCA(t, key)
	csr, err := NewX509CSR(key, WithX509CSRCommonName("path-length subject"))
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		value   int
		present bool
		want    int
	}{
		{name: "omitted", want: -1},
		{name: "explicit_unbounded", value: -1, present: true, want: -1},
		{name: "zero", value: 0, present: true, want: 0},
		{name: "one", value: 1, present: true, want: 1},
		{name: "three", value: 3, present: true, want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("direct", func(t *testing.T) {
				opts := []X509CertOption{WithX509CertCommonName("path-length CA"), WithX509CertIsCA()}
				if tc.present {
					opts = append(opts, WithX509CertCaMaxPathLen(tc.value))
				}
				der, err := NewX509Cert(key, opts...)
				require.NoError(t, err)
				cert := svidTestParse(t, der)
				require.NoError(t, cert.CheckSignatureFrom(cert))
				requirePathLenDER(t, cert, tc.want)
			})
			t.Run("CSR_signer", func(t *testing.T) {
				opts := []SignCSROption{WithX509SignCSRIsCA()}
				if tc.present {
					opts = append(opts, WithX509CaMaxPathLen(tc.value))
				}
				der, err := NewX509CertByCSR(issuer, key, csr, opts...)
				require.NoError(t, err)
				cert := svidTestParse(t, der)
				require.NoError(t, cert.CheckSignatureFrom(issuer))
				requirePathLenDER(t, cert, tc.want)
			})
		})
	}
}

// TestX509PathLenInvalidTemplates rejects constraints on non-CAs and values below the unbounded sentinel.
func TestX509PathLenInvalidTemplates(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	issuer := newCSRSignatureTestCA(t, key)
	csr, err := NewX509CSR(key, WithX509CSRCommonName("path-length subject"))
	require.NoError(t, err)
	for _, value := range []int{0, 1} {
		t.Run(fmt.Sprintf("non_CA_%d", value), func(t *testing.T) {
			der, err := NewX509Cert(key, WithX509CertCommonName("not a CA"), WithX509CertCaMaxPathLen(value))
			require.ErrorContains(t, err, "only CAs are allowed to specify MaxPathLen")
			require.Nil(t, der)
			der, err = NewX509CertByCSR(issuer, key, csr, WithX509CaMaxPathLen(value))
			require.ErrorContains(t, err, "only CAs are allowed to specify MaxPathLen")
			require.Nil(t, der)
		})
	}
	der, err := NewX509Cert(key, WithX509CertCommonName("invalid CA"), WithX509CertIsCA(), WithX509CertCaMaxPathLen(-2))
	require.ErrorContains(t, err, "invalid MaxPathLen")
	require.Nil(t, der)
	der, err = NewX509CertByCSR(issuer, key, csr, WithX509SignCSRIsCA(), WithX509CaMaxPathLen(-2))
	require.ErrorContains(t, err, "invalid MaxPathLen")
	require.Nil(t, der)
}

// pathLenTestChain issues a constrained root, depth unconstrained intermediate CAs and a workload leaf.
func pathLenTestChain(t *testing.T, maximum, depth int) (*x509.Certificate, []*x509.Certificate) {
	t.Helper()
	rootKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	rootDER, err := NewX509Cert(rootKey, WithX509CertCommonName("path boundary root"), WithX509CertIsCA(),
		WithX509CertSeriaNumber(big.NewInt(1)), WithX509CertCaMaxPathLen(maximum))
	require.NoError(t, err)
	root := svidTestParse(t, rootDER)
	parent := root
	var parentKey crypto.Signer = rootKey
	intermediates := make([]*x509.Certificate, 0, depth)
	for i := range depth {
		key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
		der, err := NewX509Cert(parentKey, WithX509CertCommonName(fmt.Sprintf("path boundary intermediate %d", i)),
			WithX509CertIsCA(), WithX509CertSeriaNumber(big.NewInt(int64(i+2))), WithX509CertParent(parent), WithX509CertPubkey(key.Public()))
		require.NoError(t, err, "issuance alone is not complete path validation")
		cert := svidTestParse(t, der)
		require.NoError(t, cert.CheckSignatureFrom(parent))
		intermediates = append(intermediates, cert)
		parent, parentKey = cert, key
	}
	leafKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	der, err := NewX509SVID(parentKey, svidTestID, WithX509CertParent(parent), WithX509CertPubkey(leafKey.Public()),
		WithX509CertSeriaNumber(big.NewInt(100)), WithX509CertDNSNames("service.example.org"))
	require.NoError(t, err)
	leaf := svidTestParse(t, der)
	require.NoError(t, leaf.CheckSignatureFrom(parent))
	chain := []*x509.Certificate{leaf}
	for i := len(intermediates) - 1; i >= 0; i-- {
		chain = append(chain, intermediates[i])
	}
	return root, chain
}

// TestX509PathLenChainBoundary checks exact N acceptance and N+1 rejection through both verifiers.
func TestX509PathLenChainBoundary(t *testing.T) {
	for _, maximum := range []int{0, 1, 3} {
		for _, extra := range []int{0, 1} {
			depth := maximum + extra
			t.Run(fmt.Sprintf("limit_%d/depth_%d", maximum, depth), func(t *testing.T) {
				root, chain := pathLenTestChain(t, maximum, depth)
				roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
				roots.AddCert(root)
				for _, cert := range chain[1:] {
					intermediates.AddCert(cert)
				}
				_, genericErr := chain[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: "service.example.org"})
				_, verified, svidErr := VerifyX509SVID(chain, svidTestBundle("example.org", root))
				if extra == 0 {
					require.NoError(t, genericErr)
					require.NoError(t, svidErr)
					require.Len(t, verified[0], depth+2)
					return
				}
				for _, err := range []error{genericErr, svidErr} {
					require.Error(t, err)
					var invalid x509.CertificateInvalidError
					require.ErrorAs(t, err, &invalid)
					require.Equal(t, x509.TooManyIntermediates, invalid.Reason)
					require.Equal(t, root.Raw, invalid.Cert.Raw)
				}
			})
		}
	}
}
