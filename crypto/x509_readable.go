package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"time"

	"github.com/Laisky/errors/v2"

	gutils "github.com/Laisky/go-utils/v6"
)

// ReadableX509Cert convert x509 certificate to readable jsonable map
func ReadableX509Cert(cert *x509.Certificate) (map[string]any, error) {
	pubkey, err := Pubkey2Pem(cert.PublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "convert public key to pem")
	}

	v := map[string]any{
		"subject":                 ReadablePkixName(cert.Subject),
		"issuer":                  ReadablePkixName(cert.Issuer),
		"subject_key_id_base64":   gutils.EncodeByBase64(cert.SubjectKeyId),
		"authority_key_id_base64": gutils.EncodeByBase64(cert.AuthorityKeyId),
		"signature_algorithm":     cert.SignatureAlgorithm.String(),
		"public_key_algorithm":    cert.PublicKeyAlgorithm.String(),
		"not_before":              cert.NotBefore.Format(time.RFC3339),
		"not_after":               cert.NotAfter.Format(time.RFC3339),
		"key_usage":               ReadableX509KeyUsage(cert.KeyUsage),
		"ext_key_usage":           ReadableX509ExtKeyUsage(cert.ExtKeyUsage),
		"is_ca":                   fmt.Sprintf("%t", cert.IsCA),
		"serial_number":           cert.SerialNumber.String(),
		"public_key":              string(pubkey),
		"sans": map[string]any{
			"dns_names":       cert.DNSNames,
			"email_addresses": cert.EmailAddresses,
			"ip_addresses":    cert.IPAddresses,
			"uris":            cert.URIs,
		},
		"ocsps":              cert.OCSPServer,
		"cris":               cert.CRLDistributionPoints,
		"policy_identifiers": ReadableOIDs(cert.PolicyIdentifiers),
	}
	return gutils.RemoveEmptyVal(v), nil
}

// ReadableX509CSR convert x509 certificate request to readable jsonable map
func ReadableX509CSR(csr *x509.CertificateRequest) (map[string]any, error) {
	pubkey, err := Pubkey2Pem(csr.PublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "convert public key to pem")
	}

	v := map[string]any{
		"subject":              ReadablePkixName(csr.Subject),
		"signature_algorithm":  csr.SignatureAlgorithm.String(),
		"public_key_algorithm": csr.PublicKeyAlgorithm.String(),
		"public_key":           string(pubkey),
		"sans": map[string]any{
			"dns_names":       csr.DNSNames,
			"email_addresses": csr.EmailAddresses,
			"ip_addresses":    csr.IPAddresses,
			"uris":            csr.URIs,
		},
	}
	return gutils.RemoveEmptyVal(v), nil
}

// ReadableX509Extention convert x509 certificate extension to readable jsonable map
func ReadableX509Extention(ext *pkix.Extension) (map[string]any, error) {
	v := map[string]any{
		"oid":           ext.Id.String(),
		"critical":      fmt.Sprintf("%t", ext.Critical),
		"raw_value_b64": gutils.EncodeByBase64(ext.Value),
	}
	return gutils.RemoveEmptyVal(v), nil
}

// ReadableX509KeyUsage convert x509 certificate key usages to readable strings
func ReadableX509KeyUsage(usage x509.KeyUsage) (usageNames []string) {
	for name, u := range map[string]x509.KeyUsage{
		"DigitalSignature":  x509.KeyUsageDigitalSignature,
		"ContentCommitment": x509.KeyUsageContentCommitment,
		"KeyEncipherment":   x509.KeyUsageKeyEncipherment,
		"DataEncipherment":  x509.KeyUsageDataEncipherment,
		"KeyAgreement":      x509.KeyUsageKeyAgreement,
		"CertSign":          x509.KeyUsageCertSign,
		"CRLSign":           x509.KeyUsageCRLSign,
		"EncipherOnly":      x509.KeyUsageEncipherOnly,
		"DecipherOnly":      x509.KeyUsageDecipherOnly,
	} {
		if usage&u != 0 {
			usageNames = append(usageNames, name)
		}
	}

	return usageNames
}

// ReadablePkixName convert pkix.Name to readable map with strings
func ReadablePkixName(name pkix.Name) map[string]any {
	m := map[string]any{
		"country":             name.Country,
		"organization":        name.Organization,
		"organizational_unit": name.OrganizationalUnit,
		"locality":            name.Locality,
		"province":            name.Province,
		"street_address":      name.StreetAddress,
		"postal_code":         name.PostalCode,
		"serial_number":       name.SerialNumber,
		"common_name":         name.CommonName,
	}

	return gutils.RemoveEmptyVal(m)
}

// ReadableX509ExtKeyUsage convert x509 certificate ext key usages to readable strings
func ReadableX509ExtKeyUsage(usages []x509.ExtKeyUsage) (usageNames []string) {
	for _, u1 := range usages {
		for name, u2 := range map[string]x509.ExtKeyUsage{
			"Any":                            x509.ExtKeyUsageAny,
			"ServerAuth":                     x509.ExtKeyUsageServerAuth,
			"ClientAuth":                     x509.ExtKeyUsageClientAuth,
			"CodeSigning":                    x509.ExtKeyUsageCodeSigning,
			"EmailProtection":                x509.ExtKeyUsageEmailProtection,
			"IPSECEndSystem":                 x509.ExtKeyUsageIPSECEndSystem,
			"IPSECTunnel":                    x509.ExtKeyUsageIPSECTunnel,
			"IPSECUser":                      x509.ExtKeyUsageIPSECUser,
			"TimeStamping":                   x509.ExtKeyUsageTimeStamping,
			"OCSPSigning":                    x509.ExtKeyUsageOCSPSigning,
			"MicrosoftServerGatedCrypto":     x509.ExtKeyUsageMicrosoftServerGatedCrypto,
			"NetscapeServerGatedCrypto":      x509.ExtKeyUsageNetscapeServerGatedCrypto,
			"MicrosoftCommercialCodeSigning": x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
			"MicrosoftKernelCodeSigning":     x509.ExtKeyUsageMicrosoftKernelCodeSigning,
		} {
			if u1 == u2 {
				usageNames = append(usageNames, name)
				break
			}
		}
	}

	return usageNames
}

// ReadableOIDs converts oids to their dotted-decimal string form (for example "1.2.3.4"),
// preserving order. It returns nil when oids is empty.
func ReadableOIDs(oids []asn1.ObjectIdentifier) (names []string) {
	for i := range oids {
		names = append(names, oids[i].String())
	}

	return names
}
