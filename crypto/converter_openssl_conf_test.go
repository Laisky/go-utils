package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

func TestX509Cert2OpensslConf(t *testing.T) {
	t.Parallel()

	t.Run("ca", func(t *testing.T) {
		t.Parallel()

		cert := &x509.Certificate{
			Subject: pkix.Name{
				CommonName:         "example.com",
				Province:           []string{"California"},
				Locality:           []string{"San Francisco"},
				Organization:       []string{"Acme Corp"},
				OrganizationalUnit: []string{"IT"},
			},
			IsCA:              true,
			PolicyIdentifiers: []asn1.ObjectIdentifier{[]int{2, 5, 29, 32}},
			DNSNames: []string{
				"localhost",
				"example.com",
			},
			IPAddresses: []net.IP{
				net.ParseIP("1.2.3.4"),
			},
		}

		expected := gutils.Dedent(`
			[ req ]
			distinguished_name = req_distinguished_name
			prompt = no
			string_mask = utf8only
			x509_extensions = v3_ca
			req_extensions = req_ext

			[ req_distinguished_name ]
			commonName = example.com
			stateOrProvinceName = California
			localityName = San Francisco
			organizationName = Acme Corp
			organizationalUnitName = IT

			[ v3_ca ]
			basicConstraints = critical, CA:TRUE
			keyUsage = cRLSign, keyCertSign
			subjectKeyIdentifier = hash
			authorityKeyIdentifier = keyid:always, issuer
			certificatePolicies = @policy-0

			[ policy-0 ]
			policyIdentifier = 2.5.29.32

			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]
			DNS.1 = localhost
			DNS.2 = example.com
			IP.1 = 1.2.3.4
			`)

		expected += "\n"

		opensslConf := X509Cert2OpensslConf(cert)
		t.Logf("got\n%s", string(opensslConf))
		require.Equal(t, expected, string(opensslConf))
	})

	t.Run("not ca", func(t *testing.T) {
		t.Parallel()

		cert := &x509.Certificate{
			Subject: pkix.Name{
				CommonName:         "example.com",
				Country:            []string{"US"},
				Province:           []string{"California"},
				Locality:           []string{"San Francisco"},
				Organization:       []string{"Acme Corp"},
				OrganizationalUnit: []string{"IT"},
			},
			IsCA: false,
			PolicyIdentifiers: []asn1.ObjectIdentifier{
				[]int{2, 5, 29, 32},
				[]int{1, 2, 3},
			},
			DNSNames: []string{
				"localhost",
				"example.com",
			},
			IPAddresses: []net.IP{
				net.ParseIP("1.2.3.4"),
			},
		}

		expected := gutils.Dedent(`
			[ req ]
			distinguished_name = req_distinguished_name
			prompt = no
			string_mask = utf8only
			x509_extensions = v3_ca
			req_extensions = req_ext

			[ req_distinguished_name ]
			commonName = example.com
			countryName = US
			stateOrProvinceName = California
			localityName = San Francisco
			organizationName = Acme Corp
			organizationalUnitName = IT

			[ v3_ca ]
			basicConstraints = critical, CA:FALSE
			keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment, keyAgreement
			extendedKeyUsage = anyExtendedKeyUsage
			subjectKeyIdentifier = hash
			authorityKeyIdentifier = keyid:always, issuer
			certificatePolicies = @policy-0, @policy-1

			[ policy-0 ]
			policyIdentifier = 2.5.29.32
			[ policy-1 ]
			policyIdentifier = 1.2.3

			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]
			DNS.1 = localhost
			DNS.2 = example.com
			IP.1 = 1.2.3.4
			`)
		expected += "\n"

		opensslConf := X509Cert2OpensslConf(cert)
		t.Logf("got\n%s", string(opensslConf))
		require.Equal(t, expected, string(opensslConf))
	})
}

func TestX509Csr2OpensslConf(t *testing.T) {
	csr := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "example.com",
			Country:            []string{"US"},
			Province:           []string{"California"},
			Locality:           []string{"San Francisco"},
			Organization:       []string{"Acme Corp"},
			OrganizationalUnit: []string{"IT"},
		},
		DNSNames: []string{
			"localhost",
			"example.com",
		},
		IPAddresses: []net.IP{
			net.ParseIP("1.2.3.4"),
		},
	}

	expectedConf := gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only
		req_extensions = req_ext

		[ req_distinguished_name ]
		commonName = example.com
		countryName = US
		stateOrProvinceName = California
		localityName = San Francisco
		organizationName = Acme Corp
		organizationalUnitName = IT

		[ req_ext ]
		subjectAltName = @alt_names

		[ alt_names ]
		DNS.1 = localhost
		DNS.2 = example.com
		IP.1 = 1.2.3.4
		`)
	expectedConf += "\n"

	opensslConf := X509Csr2OpensslConf(csr)
	t.Logf("got\n%s", string(opensslConf))
	require.Equal(t, expectedConf, string(opensslConf))
}

// TestX509Cert2OpensslConf_ConfigInjection is a regression test for an OpenSSL
// config-injection vulnerability: a newline embedded in an attacker-influenceable
// subject/SAN field could inject arbitrary OpenSSL directives (e.g. turning a
// leaf cert into a CA). The sanitizer must strip the control characters.
func TestX509Cert2OpensslConf_ConfigInjection(t *testing.T) {
	t.Parallel()

	const injected = "\n[ v3_ca ]\nbasicConstraints = critical, CA:TRUE"

	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "evil.example.com" + injected,
		},
		IsCA: false,
		DNSNames: []string{
			"good.example.com",
			"evil-san" + injected,
		},
	}

	conf := string(X509Cert2OpensslConf(cert))
	t.Logf("got\n%s", conf)

	// the injected CA directive must NOT appear as its own line
	require.NotContains(t, conf, "\nbasicConstraints = critical, CA:TRUE\n",
		"injected basicConstraints line must be stripped")
	// the legitimate (non-CA) basicConstraints line must remain intact
	require.Contains(t, conf, "basicConstraints = critical, CA:FALSE")
	// the sanitized values should be flattened onto a single line
	require.Contains(t, conf, "commonName = evil.example.com[ v3_ca ]basicConstraints = critical, CA:TRUE")
	require.NotContains(t, conf, "\r")
}

// TestX509Cert2OpensslConf_BenignStillValid ensures sanitization does not break
// the conf produced for a normal certificate.
func TestX509Cert2OpensslConf_BenignStillValid(t *testing.T) {
	t.Parallel()

	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "example.com",
		},
		IsCA:     true,
		DNSNames: []string{"example.com"},
	}

	conf := string(X509Cert2OpensslConf(cert))
	require.Contains(t, conf, "commonName = example.com\n")
	require.Contains(t, conf, "DNS.1 = example.com\n")
	require.Contains(t, conf, "basicConstraints = critical, CA:TRUE")
}

// TestX509Csr2OpensslConf_ConfigInjection is the CSR counterpart of the
// config-injection regression test.
func TestX509Csr2OpensslConf_ConfigInjection(t *testing.T) {
	t.Parallel()

	const injected = "\n[ v3_ca ]\nbasicConstraints = critical, CA:TRUE"

	csr := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "evil.example.com" + injected,
		},
		DNSNames: []string{
			"good.example.com",
			"evil-san" + injected,
		},
	}

	conf := string(X509Csr2OpensslConf(csr))
	t.Logf("got\n%s", conf)

	// the injected CA directive must NOT appear as its own line
	require.NotContains(t, conf, "\nbasicConstraints = critical, CA:TRUE\n",
		"injected basicConstraints line must be stripped")
	require.NotContains(t, conf, "\n[ v3_ca ]\n",
		"injected v3_ca section must be stripped")
	// the sanitized values should be flattened onto a single line
	require.Contains(t, conf, "commonName = evil.example.com[ v3_ca ]basicConstraints = critical, CA:TRUE")
	require.NotContains(t, conf, "\r")

	// benign CommonName still produces the expected line
	benign := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "example.com"},
		DNSNames: []string{"example.com"},
	}
	require.Contains(t, string(X509Csr2OpensslConf(benign)), "commonName = example.com\n")
}

func TestX509SignCsrOptions2OpensslConf(t *testing.T) {
	t.Parallel()

	t.Run("normal", func(t *testing.T) {
		opts := []SignCSROption{
			WithX509SignCSRIsCA(),
			WithX509SignCSRKeyUsage(x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment),
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth),
			WithX509SignCSRPolicies(
				asn1.ObjectIdentifier{1, 2, 3, 4, 5},
				asn1.ObjectIdentifier{2, 23, 140, 1, 2, 1},
			),
		}

		_, opensslConf, err := x509SignCsrOptions2OpensslConf(opts...)
		require.NoError(t, err)

		expectedConf := []byte(gutils.Dedent(`
		[req]
		x509_extensions = v3_ca

		[ v3_ca ]
		subjectKeyIdentifier = hash
		authorityKeyIdentifier = keyid:always, issuer
		basicConstraints = critical, CA:TRUE
		keyUsage = digitalSignature, keyEncipherment, keyCertSign, cRLSign
		extendedKeyUsage = serverAuth, clientAuth
		certificatePolicies = @policy-0, @policy-1

		[ policy-0 ]
		policyIdentifier = 1.2.3.4.5
		[ policy-1 ]
		policyIdentifier = 2.23.140.1.2.1
	`))
		expectedConf = append(expectedConf, '\n')

		require.Equal(t, string(expectedConf), string(opensslConf))
	})

	t.Run("any ext key usages", func(t *testing.T) {
		opts := []SignCSROption{
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageAny),
		}

		_, opensslConf, err := x509SignCsrOptions2OpensslConf(opts...)
		require.NoError(t, err)

		expectedConf := []byte(gutils.Dedent(`
			[req]
			x509_extensions = v3_ca

			[ v3_ca ]
			subjectKeyIdentifier = hash
			authorityKeyIdentifier = keyid:always, issuer
			basicConstraints = critical, CA:FALSE
			keyUsage = digitalSignature, keyEncipherment
			extendedKeyUsage = anyExtendedKeyUsage
		`))
		expectedConf = append(expectedConf, '\n')

		require.Equal(t, string(expectedConf), string(opensslConf))
	})
}
