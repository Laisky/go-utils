package crypto

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
)

// WithX509CertAllowEmptySubject opts into a DN without CN and strict SAN encoding.
// This option is Go-only. An empty leaf Subject requires a nonempty critical SAN. CA and CRL
// signing certificates still require a nonempty DN. The final Issuer DN must be nonempty,
// so an empty-Subject certificate requires a named parent. Legacy CN rules are unchanged.
func WithX509CertAllowEmptySubject() X509CertOption {
	return func(o *x509V3CertOption) error {
		o.allowEmptySubject, o.validateSAN = true, true
		return nil
	}
}

// WithX509CSRAllowEmptySubject opts into a request DN without CN and validates SAN
// encoding with the Go backend. An empty request DN requires SAN claims; the issuer must independently
// authorize all identities and make the issued SAN critical when Subject is empty.
func WithX509CSRAllowEmptySubject() X509CSROption {
	return func(o *x509CSROption) error {
		o.allowEmptySubject, o.validateSAN = true, true
		return nil
	}
}

// validateX509Issuer requires a nonempty distinguished name in the final encoded Issuer.
func validateX509Issuer(rawIssuer []byte) error {
	if bytes.Equal(rawIssuer, []byte{0x30, 0}) {
		return errors.New("certificate requires a nonempty Issuer DN; use a parent with a nonempty Subject DN")
	}
	return nil
}

// validateX509SANValues validates effective SAN input before the standard writer,
// preserving raw override precedence and avoiding nil-URI writer panics.
func validateX509SANValues(dns, email []string, ips []net.IP, uris []*url.URL, extensions []pkix.Extension) error {
	for _, ext := range extensions {
		if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) {
			return validateX509IdentitySAN(nil, extensions, false, false)
		}
	}
	var names []asn1.RawValue
	for _, value := range dns {
		names = append(names, asn1.RawValue{Class: 2, Tag: 2, Bytes: []byte(value)})
	}
	for _, value := range email {
		names = append(names, asn1.RawValue{Class: 2, Tag: 1, Bytes: []byte(value)})
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		names = append(names, asn1.RawValue{Class: 2, Tag: 7, Bytes: []byte(ip)})
	}
	for _, uri := range uris {
		if uri == nil {
			return errors.New("nil URI SAN")
		}
		names = append(names, asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte(uri.String())})
	}
	if len(names) != 0 {
		value, err := asn1.Marshal(names)
		if err != nil {
			return errors.Wrap(err, "encode SAN validation input")
		}
		extensions = append(append([]pkix.Extension(nil), extensions...), pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 17}, Value: value})
	}
	return validateX509IdentitySAN(nil, extensions, false, false)
}

// validateX509IdentitySAN checks original wire identities, duplicate extension OIDs,
// and empty-Subject requirements. Policy issuance rejects unsupported GeneralNames.
func validateX509IdentitySAN(subject []byte, extensions []pkix.Extension, requireCritical, rejectUnknown bool) error {
	seen := make(map[string]bool, len(extensions))
	var san *pkix.Extension
	for i := range extensions {
		ext := &extensions[i]
		if seen[ext.Id.String()] {
			return errors.New("duplicate X.509 extension OID")
		}
		seen[ext.Id.String()] = true
		if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) {
			san = ext
		}
	}
	emptySubject := bytes.Equal(subject, []byte{0x30, 0})
	if san == nil {
		if emptySubject {
			return errors.New("empty Subject requires nonempty SAN")
		}
		return nil
	}
	if emptySubject && requireCritical && !san.Critical {
		return errors.New("empty Subject requires critical SAN")
	}
	var names []asn1.RawValue
	rest, err := asn1.Unmarshal(san.Value, &names)
	if err != nil {
		return errors.Wrap(err, "parse original SAN identities")
	}
	if len(rest) != 0 || len(names) == 0 {
		return errors.New("SAN must contain nonempty GeneralNames without trailing data")
	}
	for _, name := range names {
		if name.Class != asn1.ClassContextSpecific {
			return errors.New("SAN GeneralName must be context-specific")
		}
		if name.Tag != 1 && name.Tag != 2 && name.Tag != 6 && name.Tag != 7 {
			if rejectUnknown {
				return errors.New("identity policy supports only DNS, IP, email and URI SANs")
			}
			continue
		}
		if name.IsCompound {
			return errors.New("DNS, IP, email and URI SANs must be primitive")
		}
		value := string(name.Bytes)
		if name.Tag == 7 {
			if len(name.Bytes) != net.IPv4len && len(name.Bytes) != net.IPv6len {
				return errors.New("IP SAN must contain exactly 4 or 16 octets")
			}
			continue
		}
		for _, b := range name.Bytes {
			if b < 0x20 || b > 0x7e {
				return errors.New("SAN identity requires printable nonempty IA5 text")
			}
		}
		switch name.Tag {
		case 2:
			if !validX509DNSName(value) {
				return errors.New("invalid DNS SAN")
			}
		case 1:
			if !validX509Email(value) {
				return errors.New("email SAN requires a bare mailbox")
			}
		case 6:
			uri, err := url.Parse(value)
			if err != nil || strings.Contains(value, " ") || uri.Scheme == "" || (uri.Host == "" && uri.Path == "" && uri.Opaque == "") {
				return errors.New("URI SAN requires an absolute URI with scheme-specific content")
			}
		}
	}
	return nil
}

// validX509Email accepts a bare ASCII mailbox with a preferred DNS domain or IP literal.
func validX509Email(value string) bool {
	for _, b := range []byte(value) {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	index := strings.LastIndexByte(value, '@')
	if index < 0 {
		return false
	}
	local := value[:index]
	if len(local) == 0 {
		return false
	}
	if local[0] == '"' {
		if len(local) < 2 || local[len(local)-1] != '"' {
			return false
		}
		for i := 1; i < len(local)-1; i++ {
			if local[i] == '\\' {
				i++
				if i >= len(local)-1 {
					return false
				}
			} else if local[i] == '"' {
				return false
			}
		}
	} else {
		for _, atom := range strings.Split(local, ".") {
			if atom == "" {
				return false
			}
			for _, c := range atom {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("!#$%&'*+-/=?^_`{|}~", c)) {
					return false
				}
			}
		}
	}
	domain := value[index+1:]
	if strings.HasPrefix(domain, "[") && strings.HasSuffix(domain, "]") {
		literal := domain[1 : len(domain)-1]
		if len(literal) > 5 && strings.EqualFold(literal[:5], "IPv6:") {
			return strings.Contains(literal[5:], ":") && net.ParseIP(literal[5:]) != nil
		}
		return !strings.Contains(literal, ":") && net.ParseIP(literal).To4() != nil
	}
	return !strings.HasPrefix(domain, "*.") && validX509DNSName(domain)
}

// validX509DNSName accepts ASCII preferred-name labels and a single leftmost wildcard.
func validX509DNSName(value string) bool {
	value = strings.TrimSuffix(value, ".")
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	value = strings.TrimPrefix(value, "*.")
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}
