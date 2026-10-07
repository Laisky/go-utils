package crypto

import (
	"crypto/x509/pkix"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

// opensslConfSpecialChars lists the characters the OpenSSL configuration
// parser (NCONF default method) interprets inside a value: escapes, variable
// expansion ($var, ${var}, $(var), $section::name, $ENV::NAME), the three
// quote characters and comments. ';' is included defensively.
const opensslConfSpecialChars = "\\$\"'`#;"

// opensslSubjectAttributes lists the subject attributes the OpenSSL
// configuration builders emit, in emission order after commonName, with the
// OpenSSL attribute name and the matching pkix.Name values.
var opensslSubjectAttributes = []struct {
	name   string
	values func(pkix.Name) []string
}{
	{"countryName", func(n pkix.Name) []string { return n.Country }},
	{"stateOrProvinceName", func(n pkix.Name) []string { return n.Province }},
	{"localityName", func(n pkix.Name) []string { return n.Locality }},
	{"organizationName", func(n pkix.Name) []string { return n.Organization }},
	{"organizationalUnitName", func(n pkix.Name) []string { return n.OrganizationalUnit }},
	{"streetAddress", func(n pkix.Name) []string { return n.StreetAddress }},
	{"postalCode", func(n pkix.Name) []string { return n.PostalCode }},
}

// opensslConfEncoder encodes caller-controlled text as literal OpenSSL
// configuration values, so the configuration parser can never expand
// variables (including ${ENV::NAME}), strip quotes, interpret escapes, start a
// comment or trim padding.
type opensslConfEncoder struct {
	// legacy selects the behavior of the exported X509Cert2OpensslConf and
	// X509Csr2OpensslConf: control characters are stripped instead of rejected
	// and unsupported pkix.Name attributes are ignored. Tongsuo issuance always
	// uses the strict encoder, which rejects anything it cannot encode exactly.
	legacy bool
}

// value returns the literal configuration encoding of s. Values without
// special characters or edge whitespace are emitted unchanged; all others are
// wrapped in double quotes with backslash and double quote escaped, which the
// parser copies verbatim. It returns an error for invalid UTF-8 and, in strict
// mode, for control characters, which cannot be represented on one line.
func (e opensslConfEncoder) value(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("config value is not valid UTF-8")
	}
	if e.legacy {
		s = sanitizeOpensslConfValue(s)
	} else if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", errors.Errorf("config value %q contains control characters", s)
	}

	if s == "" || (!strings.ContainsAny(s, opensslConfSpecialChars) &&
		strings.TrimSpace(s) == s) {
		return s, nil
	}

	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := range len(s) {
		if s[i] == '\\' || s[i] == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')

	return b.String(), nil
}

// subjectLines renders the req_distinguished_name entries for name: the
// commonName line first, then each supported attribute, using one
// "N.attribute" key per value when an attribute is multi-valued so values are
// never merged. It returns an error when a value cannot be encoded or, in
// strict mode, when name carries attributes the builders cannot emit.
func (e opensslConfEncoder) subjectLines(name pkix.Name) (string, error) {
	if !e.legacy && len(name.ExtraNames) != 0 {
		return "", errors.New("subject ExtraNames are not supported by the tongsuo config builder")
	}

	cn, err := e.value(name.CommonName)
	if err != nil {
		return "", errors.Wrap(err, "common name")
	}

	var b strings.Builder
	b.WriteString("commonName = " + cn + "\n")

	for _, attr := range opensslSubjectAttributes {
		values := attr.values(name)
		for i, v := range values {
			encoded, err := e.value(v)
			if err != nil {
				return "", errors.Wrap(err, attr.name)
			}

			key := attr.name
			if len(values) > 1 {
				key = strconv.Itoa(i) + "." + attr.name
			}
			b.WriteString(key + " = " + encoded + "\n")
		}
	}

	if name.SerialNumber != "" {
		encoded, err := e.value(name.SerialNumber)
		if err != nil {
			return "", errors.Wrap(err, "serialNumber")
		}
		b.WriteString("serialNumber = " + encoded + "\n")
	}

	return b.String(), nil
}

// ia5Value validates that a SAN value is a non-empty IA5String (ASCII) and
// returns its literal encoding. It returns an error otherwise.
func (e opensslConfEncoder) ia5Value(kind, s string) (string, error) {
	if s == "" {
		return "", errors.Errorf("empty %s subject alternative name", kind)
	}
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return "", errors.Errorf("%s subject alternative name %q is not ASCII", kind, s)
		}
	}

	encoded, err := e.value(s)
	if err != nil {
		return "", errors.Wrapf(err, "%s subject alternative name", kind)
	}

	return encoded, nil
}

// altNameLines renders one alt_names entry per SAN value (DNS.N, email.N,
// IP.N, URI.N), so separators inside a value can never add entries. Email
// values "copy" and "move", which OpenSSL treats as instructions to take
// addresses from the subject, are rejected. It returns the lines, or an error
// when any value cannot be encoded exactly.
func (e opensslConfEncoder) altNameLines(dnsNames, emails []string,
	ips []net.IP, uris []*url.URL) (string, error) {
	var b strings.Builder
	for i, v := range dnsNames {
		encoded, err := e.ia5Value("DNS", v)
		if err != nil {
			return "", errors.WithStack(err)
		}
		b.WriteString("DNS." + strconv.Itoa(i+1) + " = " + encoded + "\n")
	}

	for i, v := range emails {
		if v == "copy" || v == "move" {
			return "", errors.Errorf("email subject alternative name %q is an OpenSSL keyword", v)
		}
		encoded, err := e.ia5Value("email", v)
		if err != nil {
			return "", errors.WithStack(err)
		}
		b.WriteString("email." + strconv.Itoa(i+1) + " = " + encoded + "\n")
	}

	for i, v := range ips {
		if len(v) != net.IPv4len && len(v) != net.IPv6len {
			return "", errors.Errorf("invalid IP subject alternative name %v", []byte(v))
		}
		b.WriteString("IP." + strconv.Itoa(i+1) + " = " + v.String() + "\n")
	}

	for i, v := range uris {
		if v == nil {
			return "", errors.New("nil URI subject alternative name")
		}
		encoded, err := e.ia5Value("URI", v.String())
		if err != nil {
			return "", errors.WithStack(err)
		}
		b.WriteString("URI." + strconv.Itoa(i+1) + " = " + encoded + "\n")
	}

	return b.String(), nil
}
