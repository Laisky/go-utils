package crypto

import (
	"crypto/ecdsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestNewECDSAPrikeyAndCert(t *testing.T) {
	t.Parallel()

	for _, algo := range []ECDSACurve{
		ECDSACurveP256,
		ECDSACurveP384,
		ECDSACurveP521,
	} {
		prikeyPem, certder, err := NewECDSAPrikeyAndCert(algo,
			WithX509CertIsCA(),
			WithX509CertCommonName("ca"),
		)
		require.NoError(t, err)

		prikeyi, err := Pem2Prikey(prikeyPem)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		require.Equal(t, "ca", cert.Subject.CommonName)
		require.True(t, cert.IsCA)

		prikey, ok := prikeyi.(*ecdsa.PrivateKey)
		require.True(t, ok)
		require.True(t, prikey.PublicKey.Equal(cert.PublicKey))
	}
}

func TestNewX509CSR(t *testing.T) {
	t.Parallel()

	t.Run("sign by non-ca", func(t *testing.T) {
		t.Parallel()
		prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072)
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
		require.Equal(t, x509.SHA512WithRSA, newCert.SignatureAlgorithm)
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

func newTestSeriaNo(t *testing.T) *big.Int {
	g, err := NewDefaultX509CertSerialNumGenerator()
	require.NoError(t, err)

	return big.NewInt(g.SerialNum())
}

func TestNewX509CRL(t *testing.T) {
	t.Parallel()

	t.Run("ca without crl sign key usage", func(t *testing.T) {
		t.Parallel()
		prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertIsCA())
		require.NoError(t, err)

		prikey, err := Pem2Prikey(prikeyPem)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		serialNum := newTestSeriaNo(t)

		_, err = NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: time.Now(),
					SerialNumber:   serialNum,
				},
			},
		)
		require.NoError(t, err)
	})

	prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertIsCRLCA())
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	ca, err := Der2Cert(certder)
	require.NoError(t, err)

	serialNum := newTestSeriaNo(t)

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

		crl, err := Der2CRL(crlder)
		require.NoError(t, err)

		err = VerifyCRL(ca, crl)
		require.NoError(t, err)
	})

	t.Run("crl convert", func(t *testing.T) {
		pem := CRLDer2Pem(crlder)
		gotDer, err := CRLPem2Der(pem)
		require.NoError(t, err)
		require.Equal(t, crlder, gotDer)

		crl, err := Pem2CRL(pem)
		require.NoError(t, err)
		pem2 := CRL2Pem(crl)
		require.Equal(t, pem, pem2)

		der2 := CRL2Der(crl)
		require.Equal(t, crlder, der2)
	})

}

func Test_OIDs(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	t.Run("sign ca-csr with no options", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
	t.Parallel()

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
	t.Parallel()

	t.Run("empty ext key usage", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// BenchmarkRSA_bits/2048-16         	     116	  10240150 ns/op	   27944 B/op	     221 allocs/op
// BenchmarkRSA_bits/3072-16         	      46	  25347501 ns/op	   40680 B/op	     249 allocs/op
// BenchmarkRSA_bits/4096-16         	      26	  44732755 ns/op	   46312 B/op	     249 allocs/op
func BenchmarkRSA_bits(b *testing.B) {
	prikey2048, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(b, err)
	b.Run("2048", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			csr, err := NewX509CSR(prikey2048, WithX509CSRCommonName("laisky"))
			require.NoError(b, err)
			require.NotNil(b, csr)
		}
	})

	prikey3072, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(b, err)
	b.Run("3072", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			csr, err := NewX509CSR(prikey3072, WithX509CSRCommonName("laisky"))
			require.NoError(b, err)
			require.NotNil(b, csr)
		}
	})

	prikey4096, err := NewRSAPrikey(RSAPrikeyBits4096)
	require.NoError(b, err)
	b.Run("4096", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			csr, err := NewX509CSR(prikey4096, WithX509CSRCommonName("laisky"))
			require.NoError(b, err)
			require.NotNil(b, csr)
		}
	})
}

func Test_CrossSign(t *testing.T) {
	t.Parallel()

	prikeyRootCA1Pem, rootca1Der, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
		WithX509CertCommonName("root_ca_1"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)
	prikeyRootCA1, err := Pem2Prikey(prikeyRootCA1Pem)
	require.NoError(t, err)
	rootca1, err := Der2Cert(rootca1Der)
	require.NoError(t, err)

	prikeyRootCA2Pem, rootca2Der, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertCommonName("root_ca_1"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)
	prikeyRootCA2, err := Pem2Prikey(prikeyRootCA2Pem)
	require.NoError(t, err)
	rootca2, err := Der2Cert(rootca2Der)
	require.NoError(t, err)

	interPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)

	intercsr, err := NewX509CSR(interPrikey, WithX509CSRCommonName("intermedia"))
	require.NoError(t, err)

	// use same csr to cross sign multiple intermedia certificates
	interca1Der, err := NewX509CertByCSR(rootca1, prikeyRootCA1, intercsr,
		WithX509CaMaxPathLen(0),
		WithX509SignCSRIsCA())
	require.NoError(t, err)
	interca1, err := Der2Cert(interca1Der)
	require.NoError(t, err)
	require.Equal(t, 0, interca1.MaxPathLen)
	require.True(t, interca1.MaxPathLenZero)
	interca2Der, err := NewX509CertByCSR(rootca2, prikeyRootCA2, intercsr, WithX509SignCSRIsCA())
	require.NoError(t, err)
	interca2, err := Der2Cert(interca2Der)
	require.NoError(t, err)

	// use cross-sign intermedia ca to sign leaf certificate
	leafPrikey, err := NewRSAPrikey(RSAPrikeyBits4096)
	require.NoError(t, err)
	leafCSR, err := NewX509CSR(leafPrikey, WithX509CSRCommonName("leaf"))
	require.NoError(t, err)
	leafcertDer, err := NewX509CertByCSR(interca1, interPrikey, leafCSR)
	require.NoError(t, err)
	leafCert, err := Der2Cert(leafcertDer)
	require.NoError(t, err)

	t.Run("verify by intermedia ca 1", func(t *testing.T) {
		t.Parallel()
		opt := x509.VerifyOptions{
			Roots:         x509.NewCertPool(),
			Intermediates: x509.NewCertPool(),
		}
		opt.Roots.AddCert(rootca1)
		opt.Intermediates.AddCert(interca1)
		_, err := leafCert.Verify(opt)
		require.NoError(t, err)
	})

	t.Run("verify by intermedia ca 2", func(t *testing.T) {
		t.Parallel()
		opt := x509.VerifyOptions{
			Roots:         x509.NewCertPool(),
			Intermediates: x509.NewCertPool(),
		}
		opt.Roots.AddCert(rootca2)
		opt.Intermediates.AddCert(interca2)
		_, err := leafCert.Verify(opt)
		require.NoError(t, err)
	})

	t.Run("multiple certificate path", func(t *testing.T) {
		t.Parallel()
		opt := x509.VerifyOptions{
			Roots:         x509.NewCertPool(),
			Intermediates: x509.NewCertPool(),
		}
		opt.Roots.AddCert(rootca1)
		opt.Roots.AddCert(rootca2)
		opt.Intermediates.AddCert(interca1)
		opt.Intermediates.AddCert(interca2)
		chains, err := leafCert.Verify(opt)
		require.NoError(t, err)
		require.Len(t, chains, 2)
	})
}

func TestRandomSerialNumber(t *testing.T) {
	t.Parallel()

	t.Run("goroutine", func(t *testing.T) {
		t.Parallel()
		var pool errgroup.Group

		// ctx, cancel := context.WithCancel(context.Background())
		// defer cancel()
		// gt := gutils.NewGoroutineTest(t, cancel)

		var (
			mu sync.Mutex
			ns []int64
		)

		ng, err := NewDefaultX509CertSerialNumGenerator()
		require.NoError(t, err)

		for i := 0; i < 10000; i++ {
			// select {
			// case <-ctx.Done():
			// 	require.NoError(t, ctx.Err())
			// default:
			// }

			pool.Go(func() error {
				n := ng.SerialNum()
				require.Greater(t, n, int64(0))

				mu.Lock()
				ns = append(ns, n)
				mu.Unlock()

				return nil
			})
		}

		require.NoError(t, pool.Wait())

		s := mapset.NewSet(ns...)
		require.Equal(t, len(ns), s.Cardinality())
	})
}

// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// BenchmarkRandomSerialNumber/gen-16         	  718527	      1553 ns/op	       0 B/op	       0 allocs/op
func BenchmarkRandomSerialNumber(b *testing.B) {
	ng, err := NewDefaultX509CertSerialNumGenerator()
	require.NoError(b, err)

	b.Run("gen", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ng.SerialNum()
		}
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
