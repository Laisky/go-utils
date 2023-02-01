package encrypt

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewX509CSR(t *testing.T) {
	t.Run("sign by non-ca", func(t *testing.T) {
		prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072)
		require.NoError(t, err)

		prikey, err := Pem2Prikey(prikeyPem)
		require.NoError(t, err)

		csrPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
		require.NoError(t, err)

		csrder, err := NewX509CSR(csrPrikey,
			WithX509CSRCommonName("laisky"),
			WithX509CSRSignatureAlgorithm(x509.SHA512WithRSA),
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
	)
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	csrPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)

	csrPrikeyPem, err := Prikey2Pem(csrPrikey)
	require.NoError(t, err)

	t.Run("sign ca-csr with no options", func(t *testing.T) {
		csrder, err := NewX509CSR(csrPrikey, WithX509CSRCommonName("laisky"))
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
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
	})

	t.Run("sign ca-csr with full options", func(t *testing.T) {
		csrder, err := NewX509CSR(csrPrikey,
			WithX509CSRCommonName("laisky"),
			WithX509CSRSANS("laisky.com"),
			WithX509CSRSignatureAlgorithm(x509.SHA512WithRSA),
			WithX509CSROrganization("laisky-o"),
			WithX509CSROrganizationUnit("laisky-u"),
			WithX509CSRLocality("local"),
			WithX509CSRCountry("country"),
			WithX509CSRProvince("province"),
			WithX509CSRStreetAddrs("st-1", "st-2"),
			WithX509CSRPostalCode("200233"),
		)
		require.NoError(t, err)

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
			WithX509SignCSRValidFrom(validFrom),
			WithX509SignCSRValidFor(time.Hour),
			WithX509SignCSRCRLs("crl"),
			WithX509SignCSRPolicies(asn1.ObjectIdentifier{1, 2, 3, 4}),
			WithX509SignCSROCSPServers("ocsp"),
		)
		require.NoError(t, err)

		newCert, err := Der2Cert(newCertDer)
		require.NoError(t, err)

		require.Equal(t, "laisky", newCert.Subject.CommonName)
		require.Contains(t, newCert.DNSNames, "laisky.com")
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
	})

	t.Run("set attribtues in non-ca csr", func(t *testing.T) {
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

func TestNewX509CRL(t *testing.T) {
	t.Run("ca without crl sign key usage", func(t *testing.T) {

		prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertIsCA())
		require.NoError(t, err)

		prikey, err := Pem2Prikey(prikeyPem)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		serialNum, err := RandomSerialNumber()
		require.NoError(t, err)

		_, err = NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					SerialNumber: serialNum,
				},
			},
		)
		require.ErrorContains(t, err, "issuer must have the crlSign key usage bit set")
	})

	prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertIsCRLCA())
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	ca, err := Der2Cert(certder)
	require.NoError(t, err)

	serialNum, err := RandomSerialNumber()
	require.NoError(t, err)

	var crlder []byte
	t.Run("without crl serial number", func(t *testing.T) {
		var err error
		crlder, err = NewX509CRL(ca, prikey, nil,
			[]pkix.RevokedCertificate{
				{
					SerialNumber: serialNum,
				},
			})
		require.ErrorContains(t, err, "seriaNumber is empty")
	})

	t.Run("with crl serial number", func(t *testing.T) {
		var err error
		crlder, err = NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					SerialNumber: serialNum,
				},
			},
		)
		require.NoError(t, err)
	})

	crl, err := Der2CRL(crlder)
	require.NoError(t, err)

	// t.Log(crl)

	err = VerifyCRL(ca, crl)
	require.NoError(t, err)
}

func Test_OIDs(t *testing.T) {
	a1 := asn1.ObjectIdentifier{1, 2, 3}
	a2 := asn1.ObjectIdentifier{1, 2, 3}
	a3 := asn1.ObjectIdentifier{1, 2, 3, 4}
	require.Equal(t, a1, a2)
	require.NotEqual(t, a1, a3)
	require.NotEqual(t, a2, a3)

	_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertPolicies(a1, a2),
	)
	require.NoError(t, err)

	ca, err := Der2Cert(certder)
	require.NoError(t, err)

	require.Contains(t, ca.PolicyIdentifiers, a1)
	require.Contains(t, ca.PolicyIdentifiers, a2)
	require.NotContains(t, ca.PolicyIdentifiers, a3)
	require.True(t, OIDContains(ca.PolicyIdentifiers, a1))
	require.True(t, OIDContains(ca.PolicyIdentifiers, a2))
	require.False(t, OIDContains(ca.PolicyIdentifiers, a3))
	require.True(t, OIDContains(ca.PolicyIdentifiers, asn1.ObjectIdentifier{1, 2}, MatchPrefix()))
}

func TestNewRSAPrikeyAndCert(t *testing.T) {
	t.Run("sign ca-csr with no options", func(t *testing.T) {
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky"))
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		require.Equal(t, "laisky", cert.Subject.CommonName)
		require.NotContains(t, cert.DNSNames, "laisky.com")
		require.False(t, cert.IsCA)
		require.NotContains(t, cert.Subject.Organization, "laisky-o")
		require.NotContains(t, cert.Subject.OrganizationalUnit, "laisky-u")
		require.NotContains(t, cert.Subject.Locality, "local")
		require.NotContains(t, cert.Subject.Country, "country")
		require.NotContains(t, cert.Subject.Province, "province")
		require.NotContains(t, cert.Subject.StreetAddress, "st-1")
		require.NotContains(t, cert.Subject.StreetAddress, "st-2")
		require.NotContains(t, cert.Subject.PostalCode, "200233")
		require.NotEqual(t, big.NewInt(489238432420), cert.SerialNumber)
		require.NotEqual(t, x509.KeyUsageCRLSign, cert.KeyUsage&x509.KeyUsageCRLSign)
		require.NotContains(t, cert.ExtKeyUsage, x509.ExtKeyUsageCodeSigning)
		require.NotContains(t, cert.ExtKeyUsage, x509.KeyUsageCRLSign)
		require.NotContains(t, cert.CRLDistributionPoints, "crl")
		require.NotContains(t, cert.OCSPServer, "ocsp")
		require.Empty(t, cert.PolicyIdentifiers)
	})

	t.Run("sign ca-csr with full options", func(t *testing.T) {
		validFrom := time.Unix(time.Now().Unix(), 0).UTC()
		validAt := validFrom.Add(time.Hour)

		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky"),
			WithX509CertSANS("laisky.com"),
			WithX509CertSignatureAlgorithm(x509.SHA512WithRSA),
			WithX509CertOrganization("laisky-o"),
			WithX509CertOrganizationUnit("laisky-u"),
			WithX509CertLocality("local"),
			WithX509CertCountry("country"),
			WithX509CertProvince("province"),
			WithX509CertStreetAddrs("st-1", "st-2"),
			WithX509CertPostalCode("200233"),
			WithX509CertIsCA(),
			WithX509CertIsCRLCA(),
			WithX509CertSeriaNumber(big.NewInt(489238432420)),
			WithX509CertKeyUsage(x509.KeyUsageCRLSign),
			WithX509CertExtKeyUsage(x509.ExtKeyUsageCodeSigning),
			WithX509CertValidFrom(validFrom),
			WithX509CertValidFor(time.Hour),
			WithX509CertCRLs("crl"),
			WithX509CertOCSPServers("ocsp"),
			WithX509CertPolicies(asn1.ObjectIdentifier{1, 2, 3, 4}),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		require.Equal(t, "laisky", cert.Subject.CommonName)
		require.Contains(t, cert.DNSNames, "laisky.com")
		require.True(t, cert.IsCA)
		require.Contains(t, cert.Subject.Organization, "laisky-o")
		require.Contains(t, cert.Subject.OrganizationalUnit, "laisky-u")
		require.Contains(t, cert.Subject.Locality, "local")
		require.Contains(t, cert.Subject.Country, "country")
		require.Contains(t, cert.Subject.Province, "province")
		require.Contains(t, cert.Subject.StreetAddress, "st-1")
		require.Contains(t, cert.Subject.StreetAddress, "st-2")
		require.Contains(t, cert.Subject.PostalCode, "200233")
		require.Equal(t, big.NewInt(489238432420), cert.SerialNumber)
		require.Equal(t, x509.KeyUsageCRLSign, cert.KeyUsage&x509.KeyUsageCRLSign)
		require.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageCodeSigning)
		require.Equal(t, cert.NotBefore, validFrom)
		require.Equal(t, cert.NotAfter, validAt)
		require.NotEmpty(t, cert.KeyUsage&x509.KeyUsageCRLSign)
		require.Contains(t, cert.CRLDistributionPoints, "crl")
		require.Contains(t, cert.OCSPServer, "ocsp")
		require.True(t, OIDContains([]asn1.ObjectIdentifier{{1, 2, 3, 4}}, cert.PolicyIdentifiers[0]))
	})
}

func TestReadableX509Cert(t *testing.T) {
	validFrom := time.Unix(time.Now().Unix(), 0).UTC()
	_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertCommonName("laisky"),
		WithX509CertSANS("laisky.com"),
		WithX509CertSignatureAlgorithm(x509.SHA512WithRSA),
		WithX509CertOrganization("laisky-o"),
		WithX509CertOrganizationUnit("laisky-u"),
		WithX509CertLocality("local"),
		WithX509CertCountry("country"),
		WithX509CertProvince("province"),
		WithX509CertStreetAddrs("st-1", "st-2"),
		WithX509CertPostalCode("200233"),
		WithX509CertIsCA(),
		WithX509CertIsCRLCA(),
		WithX509CertSeriaNumber(big.NewInt(489238432420)),
		WithX509CertKeyUsage(x509.KeyUsageCRLSign),
		WithX509CertExtKeyUsage(x509.ExtKeyUsageCodeSigning),
		WithX509CertValidFrom(validFrom),
		WithX509CertValidFor(time.Hour),
		WithX509CertCRLs("crl"),
		WithX509CertOCSPServers("ocsp"),
		WithX509CertPolicies(asn1.ObjectIdentifier{1, 2, 3, 4}),
	)
	require.NoError(t, err)

	cert, err := Der2Cert(certder)
	require.NoError(t, err)

	m, err := ReadableX509Cert(cert)
	require.NoError(t, err)

	require.Equal(t, "laisky", m["subject"].(map[string]any)["common_name"])
}

func Test_ExtKeyUsage(t *testing.T) {
	t.Run("empty ext key usage", func(t *testing.T) {
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     root,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		require.NoError(t, err)
	})

	t.Run("ext key usage not match", func(t *testing.T) {
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertExtKeyUsage(x509.ExtKeyUsageCodeSigning),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     root,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		require.ErrorContains(t, err, "certificate specifies an incompatible key usage")
	})

	t.Run("ext key usage match", func(t *testing.T) {
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     root,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		require.NoError(t, err)
	})

	t.Run("ext key usage match any", func(t *testing.T) {
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots: root,
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageCodeSigning,
				x509.ExtKeyUsageServerAuth,
			},
		})
		require.NoError(t, err)
	})

	t.Run("not all cert in chain match ext key usage", func(t *testing.T) {
		// new ca
		cakeyPem, caDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertIsCA(),
			WithX509CertExtKeyUsage(x509.ExtKeyUsageCodeSigning),
		)
		require.NoError(t, err)
		ca, err := Der2Cert(caDer)
		require.NoError(t, err)
		cakey, err := Pem2Prikey(cakeyPem)
		require.NoError(t, err)

		// new leaf cert
		prikey, err := NewRSAPrikey(RSAPrikeyBits3072)
		require.NoError(t, err)
		csrDer, err := NewX509CSR(prikey)
		require.NoError(t, err)
		certDer, err := NewX509CertByCSR(ca, cakey, csrDer,
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)
		cert, err := Der2Cert(certDer)
		require.NoError(t, err)
		prikeyPem, err := Prikey2Pem(prikey)
		require.NoError(t, err)
		require.NoError(t, VerifyCertByPrikey(CertDer2Pem(certDer), prikeyPem))

		// verify
		root := x509.NewCertPool()
		root.AddCert(ca)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots: root,
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
			},
		})
		require.ErrorContains(t, err, "certificate specifies an incompatible key usage")
	})

	t.Run("all cert in chain match ext key usage", func(t *testing.T) {
		// new ca
		cakeyPem, caDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertIsCA(),
		)
		require.NoError(t, err)
		ca, err := Der2Cert(caDer)
		require.NoError(t, err)
		cakey, err := Pem2Prikey(cakeyPem)
		require.NoError(t, err)

		// new leaf cert
		prikey, err := NewRSAPrikey(RSAPrikeyBits3072)
		require.NoError(t, err)
		csrDer, err := NewX509CSR(prikey)
		require.NoError(t, err)
		certDer, err := NewX509CertByCSR(ca, cakey, csrDer,
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)
		cert, err := Der2Cert(certDer)
		require.NoError(t, err)

		// verify
		root := x509.NewCertPool()
		root.AddCert(ca)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots: root,
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
			},
		})
		require.NoError(t, err)
	})
}
