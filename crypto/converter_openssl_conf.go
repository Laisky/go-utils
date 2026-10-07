package crypto

import (
	"crypto/x509"
	"encoding/asn1"
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
// Subject and SAN values are encoded as literal configuration values (quoted
// and escaped when they contain $, quotes, backslashes, # or edge spaces), so
// OpenSSL variable expansion such as ${ENV::NAME} can never apply; for
// backward compatibility this exported helper strips control characters.
// Each SAN value becomes its own alt_names entry. Extended key usages are
// emitted exactly as requested (x509.ExtKeyUsageAny is the single
// anyExtendedKeyUsage OID); without an explicit request a non-CA certificate
// gets exactly anyExtendedKeyUsage. It returns nil when the template cannot be
// represented exactly, for example for an unknown extended key usage, a
// non-ASCII SAN or the OpenSSL email keywords "copy"/"move".
func X509Cert2OpensslConf(cert *x509.Certificate) (opensslConf []byte) {
	opensslConf, err := x509Cert2OpensslConf(cert, opensslConfEncoder{legacy: true})
	if err != nil {
		return nil
	}

	return opensslConf
}

// x509Cert2OpensslConf marshals a certificate template into OpenSSL
// configuration using enc for every caller-controlled value. It returns the
// configuration, or an error when the template cannot be represented exactly.
func x509Cert2OpensslConf(cert *x509.Certificate, enc opensslConfEncoder) (opensslConf []byte, err error) {
	extKeyUsages, err := tongsuoCertExtKeyUsages(cert)
	if err != nil {
		return nil, errors.Wrap(err, "ext key usage")
	}
	extKeyUsageLine, err := opensslExtKeyUsageLine(extKeyUsages)
	if err != nil {
		return nil, errors.Wrap(err, "ext key usage")
	}
	subjectLines, err := enc.subjectLines(cert.Subject)
	if err != nil {
		return nil, errors.Wrap(err, "subject")
	}
	altCnt, err := enc.altNameLines(cert.DNSNames, cert.EmailAddresses, cert.IPAddresses, cert.URIs)
	if err != nil {
		return nil, errors.Wrap(err, "subject alternative names")
	}

	// set req & req_distinguished_name
	cnt := gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only
		x509_extensions = v3_ca

		[ req_distinguished_name ]`)
	cnt += "\n" + subjectLines + "\n"

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
	cnt += opensslPoliciesLines(cert.PolicyIdentifiers)

	// set req_ext
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

// opensslPoliciesLines renders a certificatePolicies line and one policy
// section per OID, or an empty string when there are no policies. OIDs are
// rendered in dotted form, which needs no encoding.
func opensslPoliciesLines(policies []asn1.ObjectIdentifier) string {
	if len(policies) == 0 {
		return ""
	}

	refs := make([]string, 0, len(policies))
	var sections string
	for i, policy := range policies {
		refs = append(refs, fmt.Sprintf("@policy-%d", i))
		sections += fmt.Sprintf("[ policy-%d ]\npolicyIdentifier = %s\n", i, policy.String())
	}

	return "certificatePolicies = " + strings.Join(refs, ", ") + "\n\n" + sections
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
//
// Values are encoded exactly like X509Cert2OpensslConf. It returns nil when the
// request cannot be represented exactly.
func X509Csr2OpensslConf(csr *x509.CertificateRequest) (opensslConf []byte) {
	opensslConf, err := x509Csr2OpensslConf(csr, opensslConfEncoder{legacy: true})
	if err != nil {
		return nil
	}

	return opensslConf
}

// x509Csr2OpensslConf marshals a CSR template into OpenSSL configuration using
// enc for every caller-controlled value. It returns the configuration, or an
// error when the template cannot be represented exactly.
func x509Csr2OpensslConf(csr *x509.CertificateRequest, enc opensslConfEncoder) (opensslConf []byte, err error) {
	subjectLines, err := enc.subjectLines(csr.Subject)
	if err != nil {
		return nil, errors.Wrap(err, "subject")
	}
	sansCnt, err := enc.altNameLines(csr.DNSNames, csr.EmailAddresses, csr.IPAddresses, csr.URIs)
	if err != nil {
		return nil, errors.Wrap(err, "subject alternative names")
	}

	// set req & req_distinguished_name
	cnt := gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only

		[ req_distinguished_name ]`)
	cnt += "\n" + subjectLines

	// set req_ext
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

	return []byte(cnt), nil
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

	cnt += opensslPoliciesLines(opt.policies)

	return opt, []byte(cnt), nil
}
