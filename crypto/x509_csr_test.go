package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewX509CSR(t *testing.T) {
	t.Parallel()

	t.Run("sign by non-ca", func(t *testing.T) {
		t.Parallel()
		prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
		)
		require.NoError(t, err)

		prikey, err := Pem2Prikey(prikeyPem)
		require.NoError(t, err)

		csrPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
		require.NoError(t, err)

		csrder, err := NewX509CSR(csrPrikey,
			WithX509CSRCommonName("laisky"),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		_, err = NewX509CertByCSR(ca, prikey, csrder,
			WithX509SignCSRIsCA(),
		)
		require.Error(t, err)
	})

	// generate root-ca
	prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertIsCA(),
		WithX509CertCommonName("ca"),
		WithX509CertCaMaxPathLen(0),
	)
	require.NoError(t, err)

	ca, err := Der2Cert(certder)
	require.NoError(t, err)
	require.Equal(t, 0, ca.MaxPathLen)
	require.True(t, ca.MaxPathLenZero)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	csrPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)

	csrPrikeyPem, err := Prikey2Pem(csrPrikey)
	require.NoError(t, err)

	t.Run("sign ca-csr with no options", func(t *testing.T) {
		t.Parallel()
		csrder, err := NewX509CSR(csrPrikey,
			WithX509CSRCommonName("laisky"),
		)
		require.NoError(t, err)

		validFrom := time.Now().UTC()
		validAt := validFrom.Add(time.Hour)

		newCertDer, err := NewX509CertByCSR(ca, prikey, csrder)
		require.NoError(t, err)

		newCert, err := Der2Cert(newCertDer)
		require.NoError(t, err)

		require.Equal(t, "laisky", newCert.Subject.CommonName)
		require.NotContains(t, newCert.DNSNames, "laisky.com")
		require.False(t, newCert.IsCA)
		require.Equal(t, "ca", newCert.Issuer.CommonName)
		require.NotContains(t, newCert.Subject.Organization, "laisky-o")
		require.NotContains(t, newCert.Subject.OrganizationalUnit, "laisky-u")
		require.NotContains(t, newCert.Subject.Locality, "local")
		require.NotContains(t, newCert.Subject.Country, "country")
		require.NotContains(t, newCert.Subject.Province, "province")
		require.NotContains(t, newCert.Subject.StreetAddress, "st-1")
		require.NotContains(t, newCert.Subject.StreetAddress, "st-2")
		require.NotContains(t, newCert.Subject.PostalCode, "200233")
		require.NotEqual(t, big.NewInt(489238432420), newCert.SerialNumber)
		require.NotEqual(t, x509.KeyUsageCRLSign, newCert.KeyUsage&x509.KeyUsageCRLSign)
		require.NotContains(t, newCert.ExtKeyUsage, x509.ExtKeyUsageCodeSigning)
		require.NotEqual(t, newCert.NotBefore, validFrom)
		require.NotEqual(t, newCert.NotAfter, validAt)
		require.NotContains(t, newCert.ExtKeyUsage, x509.KeyUsageCRLSign)
		require.NotContains(t, newCert.CRLDistributionPoints, "crl")
		require.NotContains(t, newCert.OCSPServer, "ocsp")
		require.Empty(t, newCert.PolicyIdentifiers)
		require.LessOrEqual(t, newCert.MaxPathLen, 0)
		require.False(t, newCert.MaxPathLenZero)
	})

	t.Run("sign ca-csr with full options", func(t *testing.T) {
		t.Parallel()
		ext := pkix.Extension{
			Id:       asn1.ObjectIdentifier{1, 2, 3, 4, 5},
			Critical: false,
			Value:    []byte("laisky-ext"),
		}
		exext := pkix.Extension{
			Id:       asn1.ObjectIdentifier{1, 2, 3, 4, 5, 1},
			Critical: false,
			Value:    []byte("laisky-exext"),
		}

		csrder, err := NewX509CSR(csrPrikey,
			WithX509CSRCommonName("laisky"),
			WithX509CSRSANS("laisky.com"),
			WithX509CSROrganization("laisky-o"),
			WithX509CSROrganizationUnit("laisky-u"),
			WithX509CSRLocality("local"),
			WithX509CSRCountry("country"),
			WithX509CSRProvince("province"),
			WithX509CSRStreetAddrs("st-1", "st-2"),
			WithX509CSRPostalCode("200233"),
			WithX509CSRSignatureAlgorithm(x509.SHA512WithRSA),
			WithX509CSRAttribute(pkix.AttributeTypeAndValueSET{
				Type: asn1.ObjectIdentifier{1, 2, 3, 4, 5},
				Value: [][]pkix.AttributeTypeAndValue{{{
					Type:  asn1.ObjectIdentifier{1, 2, 3, 4, 5},
					Value: "laisky",
				}}},
			}),
			WithX509CSRExtension(ext),
			WithX509CSRExtraExtension(exext),
			WithX509CSRPublicKeyAlgorithm(x509.RSA),
			WithX509CSRDNSNames("laisky.com"),
			WithX509CSRIPAddrs(net.ParseIP("1.2.3.4")),
			WithX509CSRURIs(&url.URL{Scheme: "https", Host: "laisky.com"}),
		)
		require.NoError(t, err)

		csr, err := Der2CSR(csrder)
		require.NoError(t, err)
		require.Equal(t, "laisky", csr.Subject.CommonName)
		require.Contains(t, csr.Extensions, exext)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		validFrom := time.Unix(time.Now().Unix(), 0).UTC()
		validAt := validFrom.Add(time.Hour)

		newCertDer, err := NewX509CertByCSR(ca, prikey, csrder,
			WithX509SignCSRIsCA(),
			WithX509SignCSRIsCRLCA(),
			WithX509SignCSRSeriaNumber(big.NewInt(489238432420)),
			WithX509SignCSRKeyUsage(x509.KeyUsageCRLSign),
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageCodeSigning),
			WithX509SignCSRNotBefore(validFrom),
			WithX509SignCSRNotAfter(validFrom.Add(time.Hour)),
			WithX509SignCSRCRLs("crl"),
			WithX509SignCSRPolicies(asn1.ObjectIdentifier{1, 2, 3, 4}),
			WithX509SignCSROCSPServers("ocsp"),
			WithX509SignCSRExtenstions(ext),
			WithX509SignCSRExtraExtenstions(exext),
		)
		require.NoError(t, err)

		newCert, err := Der2Cert(newCertDer)
		require.NoError(t, err)

		v := net.ParseIP("1.2.3.4")
		t.Logf("%v", v)

		require.Equal(t, "laisky", newCert.Subject.CommonName)
		require.True(t, newCert.IsCA)
		require.Equal(t, "ca", newCert.Issuer.CommonName)
		require.Contains(t, newCert.Subject.Organization, "laisky-o")
		require.Contains(t, newCert.Subject.OrganizationalUnit, "laisky-u")
		require.Contains(t, newCert.Subject.Locality, "local")
		require.Contains(t, newCert.Subject.Country, "country")
		require.Contains(t, newCert.Subject.Province, "province")
		require.Contains(t, newCert.Subject.StreetAddress, "st-1")
		require.Contains(t, newCert.Subject.StreetAddress, "st-2")
		require.Contains(t, newCert.Subject.PostalCode, "200233")
		require.Equal(t, big.NewInt(489238432420), newCert.SerialNumber)
		require.Equal(t, x509.KeyUsageCRLSign, newCert.KeyUsage&x509.KeyUsageCRLSign)
		require.Contains(t, newCert.ExtKeyUsage, x509.ExtKeyUsageCodeSigning)
		require.Equal(t, newCert.NotBefore, validFrom)
		require.Equal(t, newCert.NotAfter, validAt)
		require.NotEmpty(t, newCert.KeyUsage&x509.KeyUsageCRLSign)
		require.Contains(t, newCert.CRLDistributionPoints, "crl")
		require.Contains(t, newCert.OCSPServer, "ocsp")
		require.True(t, OIDContains([]asn1.ObjectIdentifier{{1, 2, 3, 4}}, newCert.PolicyIdentifiers[0]))
		require.Equal(t, x509.SHA256WithRSA, newCert.SignatureAlgorithm)
		require.Equal(t, x509.RSA, newCert.PublicKeyAlgorithm)
		require.Contains(t, newCert.DNSNames, "laisky.com")
		require.True(t, newCert.IPAddresses[0].Equal(net.ParseIP("1.2.3.4")))
		require.Contains(t, newCert.URIs, &url.URL{Scheme: "https", Host: "laisky.com"})
		require.Contains(t, newCert.Extensions, exext)
		// require.Contains(t, newCert.ExtraExtensions, exext)
	})

	t.Run("set attribtues in non-ca csr", func(t *testing.T) {
		t.Parallel()
		csrder, err := NewX509CSR(csrPrikey,
			WithX509CSRCommonName("laisky"),
			WithX509CSRSANS("laisky.com"),
			WithX509CSRSignatureAlgorithm(x509.SHA512WithRSA),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		newCertDer, err := NewX509CertByCSR(ca, prikey, csrder)
		require.NoError(t, err)

		newCert, err := Der2Cert(newCertDer)
		require.NoError(t, err)

		require.Equal(t, "laisky", newCert.Subject.CommonName)
		require.Contains(t, newCert.DNSNames, "laisky.com")
		require.False(t, newCert.IsCA)

		t.Run("verify", func(t *testing.T) {
			roots := x509.NewCertPool()
			roots.AppendCertsFromPEM(CertDer2Pem(certder))
			_, err = newCert.Verify(x509.VerifyOptions{
				Roots:     roots,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			})
			require.NoError(t, err)

			err = VerifyCertByPrikey(CertDer2Pem(newCertDer), csrPrikeyPem)
			require.NoError(t, err)
		})
	})
}

func TestReadableX509CSR(t *testing.T) {
	t.Parallel()

	prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
	require.NoError(t, err)

	csrder, err := NewX509CSR(prikey, WithX509CSRCommonName("test"))
	require.NoError(t, err)

	csr, err := Der2CSR(csrder)
	require.NoError(t, err)

	got, err := ReadableX509CSR(csr)
	require.NoError(t, err)

	require.Equal(t, "test", got["subject"].(map[string]any)["common_name"])

}
