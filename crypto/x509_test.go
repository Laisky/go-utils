package crypto

import (
	"crypto/ecdsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/asn1"
	"math/big"
	"sync"
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestCrossAlgorithmSign verifies that an RSA-2048 root CA can issue a certificate for a P-256
// ECDSA CSR via NewX509CertByCSR, and that the resulting ECDSA leaf certificate verifies against a
// pool containing only that RSA root.
func TestCrossAlgorithmSign(t *testing.T) {
	rootcaPrikeyPem, rootcaCertDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
		WithX509CertCommonName("rootca"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)

	rootcaPrikey, err := Pem2Prikey(rootcaPrikeyPem)
	require.NoError(t, err)
	rootca, err := Der2Cert(rootcaCertDer)
	require.NoError(t, err)

	ecPrikey, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)

	ecCsrDer, err := NewX509CSR(ecPrikey,
		WithX509CSRCommonName("ec-leaf"),
	)
	require.NoError(t, err)

	csr, err := Der2CSR(ecCsrDer)
	require.NoError(t, err, csr)

	// sign ec leaf cert by rsa rootca
	leafCertDer, err := NewX509CertByCSR(rootca, rootcaPrikey, ecCsrDer) // WithX509SignSignatureAlgorithm(x509.SHA384WithRSA),

	require.NoError(t, err)

	// verify cert
	leafCert, err := Der2Cert(leafCertDer)
	require.NoError(t, err)

	roots := x509.NewCertPool()
	roots.AddCert(rootca)
	_, err = leafCert.Verify(x509.VerifyOptions{
		Roots: roots,
	})
	require.NoError(t, err)
}

// TestNewECDSAPrikeyAndCert verifies NewECDSAPrikeyAndCert for the P-256, P-384 and P-521 curves:
// the returned PEM key parses to an *ecdsa.PrivateKey whose public key matches the certificate, and
// the certificate carries the "ca" common name and the CA flag.
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

// TestX509CertSubjectKeyID verifies that X509CertSubjectKeyID returns the SHA-1 digest of the
// public key's PKIX DER encoding (as produced by Pubkey2Der) for an RSA-2048 key.
func TestX509CertSubjectKeyID(t *testing.T) {
	t.Parallel()

	prikey, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)

	pubkeyDer, err := Pubkey2Der(&prikey.PublicKey)
	require.NoError(t, err)

	expected := sha1.Sum(pubkeyDer)

	got, err := X509CertSubjectKeyID(&prikey.PublicKey)
	require.NoError(t, err)
	require.Equal(t, expected[:], got)
}

// TestNewRSAPrikeyAndCert verifies the certificate produced by NewRSAPrikeyAndCert with RSA-3072
// keys. With only a common name, none of the optional subject fields, SANs, CA flag, custom serial
// number, extra key usages, CRL or OCSP endpoints, or policies are present; with the full option
// set, every one of them, including the validity window and the policy OID in both
// PolicyIdentifiers and Policies, is reflected in the parsed certificate.
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
		require.NotEmpty(t, cert.PolicyIdentifiers)
		require.True(t, OIDContains([]asn1.ObjectIdentifier{{1, 2, 3, 4}}, cert.PolicyIdentifiers[0]))
		oid, err := OidAsn2X509(asn1.ObjectIdentifier{1, 2, 3, 4})
		require.NoError(t, err)
		require.Contains(t, cert.Policies, oid)
	})
}

// BenchmarkRSA_bits measures NewX509CSR when creating a CSR with only a common name from
// pre-generated RSA keys of 2048, 3072 and 4096 bits, with one sub-benchmark per key size.
//
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

// Test_CrossSign verifies cross-signing: one intermediate CSR is signed as a CA by two independent
// RSA root CAs (the first with a zero max path length, which is asserted), and a leaf certificate
// issued with the intermediate key validates through either root and intermediate pair alone, and
// yields two chains when both pairs are available.
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

// TestRandomSerialNumber verifies that DefaultX509CertSerialNumGenerator is safe for concurrent use
// and produces positive, unique serial numbers across 10,000 calls made from concurrent goroutines.
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

// BenchmarkRandomSerialNumber measures the cost of a single SerialNum call on
// DefaultX509CertSerialNumGenerator.
//
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

// TestNewEd25519PrikeyAndCert verifies that NewEd25519PrikeyAndCert produces a parseable
// certificate whose common name is reported by ReadableX509Cert, and that the WithX509CertIsCA
// option sets the CA flag on the generated certificate.
func TestNewEd25519PrikeyAndCert(t *testing.T) {
	t.Parallel()

	t.Run("generate ed25519 prikey and cert", func(t *testing.T) {
		_, certDer, err := NewEd25519PrikeyAndCert(
			WithX509CertCommonName("test_common_name"),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certDer)
		require.NoError(t, err)

		got, err := ReadableX509Cert(cert)
		require.NoError(t, err)
		require.Equal(t, "test_common_name", got["subject"].(map[string]any)["common_name"])
	})

	t.Run("generate ed25519 prikey and cert with options", func(t *testing.T) {
		_, certDer, err := NewEd25519PrikeyAndCert(
			WithX509CertIsCA(),
			WithX509CertCommonName("test_common_name"),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certDer)
		require.NoError(t, err)

		require.True(t, cert.IsCA)
		require.Equal(t, "test_common_name", cert.Subject.CommonName)
	})
}
