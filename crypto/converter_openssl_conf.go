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

// X509Cert2OpensslConf marshals a certificate template into the OpenSSL
// configuration used by Tongsuo.NewX509Cert.
//
// Extended key usages are emitted exactly as requested (x509.ExtKeyUsageAny is
// the single anyExtendedKeyUsage OID); without an explicit request a non-CA
// certificate gets exactly anyExtendedKeyUsage. It returns nil when the
// template cannot be represented exactly, for example for an unknown extended
// key usage.
func X509Cert2OpensslConf(cert *x509.Certificate) (opensslConf []byte) {
	opensslConf, err := x509Cert2OpensslConf(cert)
	if err != nil {
		return nil
	}

	return opensslConf
}

// x509Cert2OpensslConf marshals a certificate template into OpenSSL
// configuration. It returns the configuration, or an error when the template
// cannot be represented exactly.
func x509Cert2OpensslConf(cert *x509.Certificate) (opensslConf []byte, err error) {
	extKeyUsages, err := tongsuoCertExtKeyUsages(cert)
	if err != nil {
		return nil, errors.Wrap(err, "ext key usage")
	}
	extKeyUsageLine, err := opensslExtKeyUsageLine(extKeyUsages)
	if err != nil {
		return nil, errors.Wrap(err, "ext key usage")
	}

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
	}
	cnt += extKeyUsageLine
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

	return []byte(cnt), nil
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
	// opensslExtKeyUsages lists every supported extended key usage, in the
	// deterministic order it is emitted, with the OpenSSL configuration token
	// that encodes exactly its OID. Tokens are OpenSSL short names that have
	// been stable for many releases, or a dotted OID when no such name exists.
	opensslExtKeyUsages = []struct {
		usage x509.ExtKeyUsage
		token string
	}{
		{x509.ExtKeyUsageAny, "anyExtendedKeyUsage"},
		{x509.ExtKeyUsageServerAuth, "serverAuth"},
		{x509.ExtKeyUsageClientAuth, "clientAuth"},
		{x509.ExtKeyUsageCodeSigning, "codeSigning"},
		{x509.ExtKeyUsageEmailProtection, "emailProtection"},
		{x509.ExtKeyUsageIPSECEndSystem, "ipsecEndSystem"},
		{x509.ExtKeyUsageIPSECTunnel, "ipsecTunnel"},
		{x509.ExtKeyUsageIPSECUser, "ipsecUser"},
		{x509.ExtKeyUsageTimeStamping, "timeStamping"},
		{x509.ExtKeyUsageOCSPSigning, "OCSPSigning"},
		{x509.ExtKeyUsageMicrosoftServerGatedCrypto, "msSGC"},
		{x509.ExtKeyUsageNetscapeServerGatedCrypto, "nsSGC"},
		{x509.ExtKeyUsageMicrosoftCommercialCodeSigning, "msCodeCom"},
		{x509.ExtKeyUsageMicrosoftKernelCodeSigning, "1.3.6.1.4.1.311.61.1.1"},
	}
)

// opensslExtKeyUsageToken returns the OpenSSL configuration token that
// encodes exactly the OID of usage, or an error for an unsupported usage.
func opensslExtKeyUsageToken(usage x509.ExtKeyUsage) (string, error) {
	for _, known := range opensslExtKeyUsages {
		if known.usage == usage {
			return known.token, nil
		}
	}

	return "", errors.Errorf("unsupported ext key usage %d", usage)
}

// canonicalExtKeyUsages validates requested extended key usages and returns
// them deduplicated in the deterministic emission order of opensslExtKeyUsages.
// x509.ExtKeyUsageAny is preserved as the single anyExtendedKeyUsage OID and is
// never expanded into concrete usages. It returns nil for an empty request and
// an error for any usage that cannot be encoded exactly, so that no requested
// usage is silently dropped.
func canonicalExtKeyUsages(requested []x509.ExtKeyUsage) ([]x509.ExtKeyUsage, error) {
	if len(requested) == 0 {
		return nil, nil
	}

	wanted := make(map[x509.ExtKeyUsage]bool, len(requested))
	for _, usage := range requested {
		if _, err := opensslExtKeyUsageToken(usage); err != nil {
			return nil, errors.WithStack(err)
		}
		wanted[usage] = true
	}

	out := make([]x509.ExtKeyUsage, 0, len(wanted))
	for _, known := range opensslExtKeyUsages {
		if wanted[known.usage] {
			out = append(out, known.usage)
		}
	}

	return out, nil
}

// opensslExtKeyUsageLine renders an extendedKeyUsage config line for the
// canonical usages, or an empty string when usages is empty. It returns an
// error if a usage has no OpenSSL token.
func opensslExtKeyUsageLine(usages []x509.ExtKeyUsage) (string, error) {
	if len(usages) == 0 {
		return "", nil
	}

	tokens := make([]string, 0, len(usages))
	for _, usage := range usages {
		token, err := opensslExtKeyUsageToken(usage)
		if err != nil {
			return "", errors.WithStack(err)
		}
		tokens = append(tokens, token)
	}

	return "extendedKeyUsage = " + strings.Join(tokens, ", ") + "\n", nil
}

// tongsuoCertExtKeyUsages returns the canonical extended key usages that
// X509Cert2OpensslConf emits for a self-signed certificate: the explicit
// request when present, otherwise the legacy default of exactly
// anyExtendedKeyUsage for non-CA certificates and none for CA certificates.
// It returns an error for usages that cannot be encoded exactly.
func tongsuoCertExtKeyUsages(cert *x509.Certificate) ([]x509.ExtKeyUsage, error) {
	if len(cert.ExtKeyUsage) != 0 {
		return canonicalExtKeyUsages(cert.ExtKeyUsage)
	}
	if cert.IsCA {
		return nil, nil
	}

	return []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, nil
}

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

	var keyUsages []string
	for _, name := range sortedKeyUsages {
		usage := keyUsagesMap[name]
		if opt.keyUsage&usage != 0 {
			keyUsages = append(keyUsages, name)
		}
	}
	if len(keyUsages) != 0 {
		cnt += fmt.Sprintf("keyUsage = %s\n", strings.Join(keyUsages, ", "))
	}

	// emit exactly the requested usages; Any stays the anyExtendedKeyUsage OID
	extKeyUsages, err := canonicalExtKeyUsages(opt.extKeyUsage)
	if err != nil {
		return nil, nil, errors.Wrap(err, "ext key usage")
	}
	extKeyUsageLine, err := opensslExtKeyUsageLine(extKeyUsages)
	if err != nil {
		return nil, nil, errors.Wrap(err, "ext key usage")
	}
	cnt += extKeyUsageLine

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
