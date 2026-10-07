package crypto

import (
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"

	gutils "github.com/Laisky/go-utils/v6"
)

// sanitizeOpensslConfValue strips all control characters (runes < 0x20 and
// 0x7f, including CR and LF) from a value before it is interpolated into the
// line-oriented OpenSSL config.
//
// Security: certificate/CSR subject and SAN fields are attacker-influenceable.
// A value containing a newline could inject arbitrary OpenSSL config directives
// (e.g. a fake "[ v3_ca ]" section turning a leaf cert into a CA). Legitimate
// X.509 subject/SAN fields never contain control characters, so stripping them
// is safe and blocks this config-injection vector.
func sanitizeOpensslConfValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// X509Cert2OpensslConf marshal x509
func X509Cert2OpensslConf(cert *x509.Certificate) (opensslConf []byte) {
	// set req & req_distinguished_name
	// sanitize the attacker-influenceable CommonName to block config injection
	cnt := fmt.Sprintf(gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only
		x509_extensions = v3_ca

		[ req_distinguished_name ]
		commonName = %s`), sanitizeOpensslConfValue(cert.Subject.CommonName))
	cnt += "\n"

	subjectMaps := map[string][]string{
		"countryName":            cert.Subject.Country,
		"stateOrProvinceName":    cert.Subject.Province,
		"localityName":           cert.Subject.Locality,
		"organizationName":       cert.Subject.Organization,
		"organizationalUnitName": cert.Subject.OrganizationalUnit,
	}

	for _, name := range []string{ // keep order
		"countryName",
		"stateOrProvinceName",
		"localityName",
		"organizationName",
		"organizationalUnitName",
	} {
		if len(subjectMaps[name]) != 0 {
			// sanitize each subject element to block config injection
			vals := make([]string, len(subjectMaps[name]))
			for i, v := range subjectMaps[name] {
				vals[i] = sanitizeOpensslConfValue(v)
			}
			cnt += fmt.Sprintf("%s = %s\n", name, strings.Join(vals, ","))
		}
	}
	cnt += "\n"

	// set v3_ca
	cnt += gutils.Dedent(`
		[ v3_ca ]
		basicConstraints = critical, CA:`)
	if cert.IsCA {
		cnt += "TRUE\nkeyUsage = cRLSign, keyCertSign\n"
	} else {
		cnt += "FALSE\nkeyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment, keyAgreement\n"
		cnt += "extendedKeyUsage = anyExtendedKeyUsage\n"
	}
	cnt += "subjectKeyIdentifier = hash\nauthorityKeyIdentifier = keyid:always, issuer\n"

	// set policies
	if len(cert.PolicyIdentifiers) > 0 {
		cnt += "certificatePolicies = "

		var policySecions string
		for i, policy := range cert.PolicyIdentifiers {
			cnt += fmt.Sprintf("@policy-%d, ", i)
			policySecions += fmt.Sprintf("[ policy-%d ]\npolicyIdentifier = %s\n", i, policy.String())
		}

		cnt = strings.TrimRight(cnt, ", ")
		cnt += "\n\n" + policySecions
	}

	// set req_ext
	// sanitize each SAN entry to block config injection via embedded newlines
	var altCnt string
	for i, v := range cert.DNSNames {
		altCnt += fmt.Sprintf("DNS.%d = %s\n", i+1, sanitizeOpensslConfValue(v))
	}
	for i, v := range cert.EmailAddresses {
		altCnt += fmt.Sprintf("email.%d = %s\n", i+1, sanitizeOpensslConfValue(v))
	}
	for i, v := range cert.IPAddresses {
		altCnt += fmt.Sprintf("IP.%d = %s\n", i+1, sanitizeOpensslConfValue(v.String()))
	}
	for i, v := range cert.URIs {
		altCnt += fmt.Sprintf("URI.%d = %s\n", i+1, sanitizeOpensslConfValue(v.String()))
	}
	if altCnt != "" {
		cnt += "\n"
		cnt += gutils.Dedent(`
			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]`)
		cnt += "\n"
		cnt += altCnt
		cnt = strings.ReplaceAll(cnt, "x509_extensions = v3_ca", "x509_extensions = v3_ca\nreq_extensions = req_ext")
	}

	return []byte(cnt)
}

// X509Csr2OpensslConf marshal x509 csr to openssl conf
//
// # Returns
//
//	[ req ]
//	distinguished_name = req_distinguished_name
//	prompt = no
//	string_mask = utf8only
//	req_extensions = req_ext
//
//	[ req_ext ]
//	subjectAltName = @alt_names
//
//	[ req_distinguished_name ]
//	commonName = Intermedia CA
//	countryName = CN
//	stateOrProvinceName = Shanghai
//	localityName = Shanghai
//	organizationName = BBT
//	organizationalUnitName = XSS
//
//	[ alt_names ]
//	DNS.1 = localhost
//	DNS.2 = example.com
func X509Csr2OpensslConf(csr *x509.CertificateRequest) (opensslConf []byte) {
	// set req & req_distinguished_name
	// sanitize the attacker-influenceable CommonName to block config injection
	cnt := fmt.Sprintf(gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only

		[ req_distinguished_name ]
		commonName = %s`), sanitizeOpensslConfValue(csr.Subject.CommonName))
	cnt += "\n"

	subjectMaps := map[string][]string{
		"countryName":            csr.Subject.Country,
		"stateOrProvinceName":    csr.Subject.Province,
		"localityName":           csr.Subject.Locality,
		"organizationName":       csr.Subject.Organization,
		"organizationalUnitName": csr.Subject.OrganizationalUnit,
	}

	for _, name := range []string{ // keep order
		"countryName",
		"stateOrProvinceName",
		"localityName",
		"organizationName",
		"organizationalUnitName",
	} {
		if len(subjectMaps[name]) != 0 {
			// sanitize each subject element to block config injection
			vals := make([]string, len(subjectMaps[name]))
			for i, v := range subjectMaps[name] {
				vals[i] = sanitizeOpensslConfValue(v)
			}
			cnt += fmt.Sprintf("%s = %s\n", name, strings.Join(vals, ","))
		}
	}

	// set req_ext
	// sanitize each SAN entry to block config injection via embedded newlines
	var sansCnt string
	for i, v := range csr.DNSNames {
		sansCnt += fmt.Sprintf("DNS.%d = %s\n", i+1, sanitizeOpensslConfValue(v))
	}
	for i, v := range csr.EmailAddresses {
		sansCnt += fmt.Sprintf("email.%d = %s\n", i+1, sanitizeOpensslConfValue(v))
	}
	for i, v := range csr.IPAddresses {
		sansCnt += fmt.Sprintf("IP.%d = %s\n", i+1, sanitizeOpensslConfValue(v.String()))
	}
	for i, v := range csr.URIs {
		sansCnt += fmt.Sprintf("URI.%d = %s\n", i+1, sanitizeOpensslConfValue(v.String()))
	}
	if sansCnt != "" {
		cnt += "\n"
		cnt += gutils.Dedent(`
			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]`)
		cnt += "\n"
		cnt += sansCnt

		cnt = strings.ReplaceAll(cnt, "string_mask = utf8only", "string_mask = utf8only\nreq_extensions = req_ext")
	}

	return []byte(cnt)
}

var (
	sortedKeyUsages = []string{
		"digitalSignature",
		"nonRepudiation",
		"keyEncipherment",
		"dataEncipherment",
		"keyAgreement",
		"keyCertSign",
		"cRLSign",
		"encipherOnly",
		"decipherOnly",
	}
	keyUsagesMap = map[string]x509.KeyUsage{
		"digitalSignature": x509.KeyUsageDigitalSignature,
		"nonRepudiation":   x509.KeyUsageContentCommitment, // nonRepudiation is also known as contentCommitment
		"keyEncipherment":  x509.KeyUsageKeyEncipherment,
		"dataEncipherment": x509.KeyUsageDataEncipherment,
		"keyAgreement":     x509.KeyUsageKeyAgreement,
		"keyCertSign":      x509.KeyUsageCertSign, // Corrected from "CertSign" to "keyCertSign"
		"cRLSign":          x509.KeyUsageCRLSign,
		"encipherOnly":     x509.KeyUsageEncipherOnly,
		"decipherOnly":     x509.KeyUsageDecipherOnly,
	}
	sortedExtKeyUsages = []string{
		"serverAuth",
		"clientAuth",
		"codeSigning",
		"emailProtection",
		"ipsecEndSystem",
		"ipsecTunnel",
		"ipsecUser",
		"timestamping",
		"ocspSigning",
		"microsoftServerGatedCrypto",
		"netscapeServerGatedCrypto",
		"microsoftCommercialCodeSigning",
		"microsoftKernelCodeSigning",
	}
	extKeyUsagesMap = map[string]x509.ExtKeyUsage{
		"serverAuth":                     x509.ExtKeyUsageServerAuth,
		"clientAuth":                     x509.ExtKeyUsageClientAuth,
		"codeSigning":                    x509.ExtKeyUsageCodeSigning,
		"emailProtection":                x509.ExtKeyUsageEmailProtection,
		"ipsecEndSystem":                 x509.ExtKeyUsageIPSECEndSystem,
		"ipsecTunnel":                    x509.ExtKeyUsageIPSECTunnel,
		"ipsecUser":                      x509.ExtKeyUsageIPSECUser,
		"timestamping":                   x509.ExtKeyUsageTimeStamping,
		"ocspSigning":                    x509.ExtKeyUsageOCSPSigning,
		"microsoftServerGatedCrypto":     x509.ExtKeyUsageMicrosoftServerGatedCrypto,
		"netscapeServerGatedCrypto":      x509.ExtKeyUsageNetscapeServerGatedCrypto,
		"microsoftCommercialCodeSigning": x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
		"microsoftKernelCodeSigning":     x509.ExtKeyUsageMicrosoftKernelCodeSigning,
	}
)

// x509SignCsrOptions2OpensslConf marshal x509 csr to openssl conf
func x509SignCsrOptions2OpensslConf(opts ...SignCSROption) (opt *signCSROption, opensslConf []byte, err error) {
	opt, err = new(signCSROption).applyOpts(nil, opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "apply options")
	}

	cnt := gutils.Dedent(`
		[req]
		x509_extensions = v3_ca

		[ v3_ca ]
		subjectKeyIdentifier = hash
		authorityKeyIdentifier = keyid:always, issuer
		basicConstraints = critical, CA:`)

	if opt.isCA {
		cnt += "TRUE\n"
	} else {
		cnt += "FALSE\n"
	}

	var extKeyUsages, keyUsages []string
	for _, name := range sortedKeyUsages {
		usage := keyUsagesMap[name]
		if opt.keyUsage&usage != 0 {
			keyUsages = append(keyUsages, name)
		}
	}
	if len(keyUsages) != 0 {
		cnt += fmt.Sprintf("keyUsage = %s\n", strings.Join(keyUsages, ", "))
	}

	if gutils.Contains(opt.extKeyUsage, x509.ExtKeyUsageAny) {
		// The main purpose of this function is to cater to the needs of non-compatible national SM2 standards.
		// Since Tongsuo does not support anyExtendedKeyUsage, so it is better to use enumeration instead.
		cnt += "extendedKeyUsage = serverAuth, clientAuth, codeSigning, emailProtection, ipsecEndSystem, " +
			"ipsecTunnel, ipsecUser, timestamping, ocspSigning, microsoftServerGatedCrypto, " +
			"netscapeServerGatedCrypto, microsoftCommercialCodeSigning, microsoftKernelCodeSigning\n"
	} else {
		for _, name := range sortedExtKeyUsages {
			usage := extKeyUsagesMap[name]
			if gutils.Contains(opt.extKeyUsage, usage) {
				extKeyUsages = append(extKeyUsages, name)
			}
		}
		if len(extKeyUsages) != 0 {
			cnt += fmt.Sprintf("extendedKeyUsage = %s\n", strings.Join(extKeyUsages, ", "))
		}
	}

	if len(opt.policies) > 0 {
		cnt += "certificatePolicies = "

		var policySecions string
		for i, policy := range opt.policies {
			cnt += fmt.Sprintf("@policy-%d, ", i)
			policySecions += fmt.Sprintf("[ policy-%d ]\npolicyIdentifier = %s\n", i, policy.String())
		}

		cnt = strings.TrimRight(cnt, ", ")
		cnt += "\n\n" + policySecions
	}

	return opt, []byte(cnt), nil
}
