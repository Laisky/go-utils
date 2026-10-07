package crypto

import (
	"crypto/x509/pkix"
	"net"
	"net/url"
	"slices"

	"github.com/Laisky/errors/v2"
)

// tongsuoNames is the subject and subject alternative names that a
// certificate or CSR generated from an OpenSSL configuration must carry.
type tongsuoNames struct {
	subject  pkix.Name
	dnsNames []string
	emails   []string
	ips      []net.IP
	uris     []*url.URL
}

// urisToStrings renders URIs for comparison; nil entries become empty strings.
func urisToStrings(uris []*url.URL) []string {
	out := make([]string, 0, len(uris))
	for _, u := range uris {
		if u == nil {
			out = append(out, "")
			continue
		}
		out = append(out, u.String())
	}

	return out
}

// subjectAttributeCount returns how many subject attributes the configuration
// builders emit for name: commonName, every supported multi-valued attribute
// value and serialNumber when set.
func subjectAttributeCount(name pkix.Name) int {
	count := 1 // commonName is always emitted
	for _, attr := range opensslSubjectAttributes {
		count += len(attr.values(name))
	}
	if name.SerialNumber != "" {
		count++
	}

	return count
}

// verify requires got to carry exactly the requested subject attributes and
// SANs, value for value and in order, with no extra subject attributes. The
// error names the first mismatching field without echoing values, so
// unexpected content never leaks through error strings.
func (n tongsuoNames) verify(got tongsuoNames) error {
	if got.subject.CommonName != n.subject.CommonName {
		return errors.New("generated common name differs from the request")
	}
	for _, attr := range opensslSubjectAttributes {
		if !slices.Equal(attr.values(n.subject), attr.values(got.subject)) {
			return errors.Errorf("generated subject %s differs from the request", attr.name)
		}
	}
	if got.subject.SerialNumber != n.subject.SerialNumber {
		return errors.New("generated subject serialNumber differs from the request")
	}
	if len(got.subject.Names) != subjectAttributeCount(n.subject) {
		return errors.New("generated subject carries unrequested attributes")
	}

	if !slices.Equal(n.dnsNames, got.dnsNames) {
		return errors.New("generated DNS names differ from the request")
	}
	if !slices.Equal(n.emails, got.emails) {
		return errors.New("generated email addresses differ from the request")
	}
	if !slices.EqualFunc(n.ips, got.ips, func(a, b net.IP) bool { return a.Equal(b) }) {
		return errors.New("generated IP addresses differ from the request")
	}
	if !slices.Equal(urisToStrings(n.uris), urisToStrings(got.uris)) {
		return errors.New("generated URIs differ from the request")
	}

	return nil
}
