package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"

	"github.com/Laisky/errors/v2"
)

// convertRevokedCertificateEntries preserves legacy entry extensions in the fields
// consumed by CreateRevocationList. The reason code has a dedicated field; zero
// has the same unspecified meaning when omitted. Other extensions remain opaque.
// CertificateIssuer is rejected because this helper does not implement indirect
// CRL semantics. Callers remain responsible for CRL scope and issuance policy.
func convertRevokedCertificateEntries(revoked []pkix.RevokedCertificate) ([]x509.RevocationListEntry, error) {
	entries := make([]x509.RevocationListEntry, len(revoked))
	for i, legacy := range revoked {
		entry := x509.RevocationListEntry{SerialNumber: legacy.SerialNumber, RevocationTime: legacy.RevocationTime}
		seen := make(map[string]struct{}, len(legacy.Extensions))
		for _, extension := range legacy.Extensions {
			oid := extension.Id.String()
			if _, exists := seen[oid]; exists {
				return nil, errors.Errorf("revoked entry %d has duplicate extension identifiers", i)
			}
			seen[oid] = struct{}{}
			switch {
			case extension.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 21}):
				if extension.Critical {
					return nil, errors.Errorf("revoked entry %d has a critical reason code", i)
				}
				var reason asn1.Enumerated
				rest, err := asn1.Unmarshal(extension.Value, &reason)
				if err != nil || len(rest) != 0 || reason < 0 || reason > 10 || reason == 7 {
					return nil, errors.Errorf("revoked entry %d has an invalid reason code", i)
				}
				entry.ReasonCode = int(reason)
			case extension.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 29}):
				return nil, errors.Errorf("revoked entry %d requires unsupported indirect CRL semantics", i)
			default:
				entry.ExtraExtensions = append(entry.ExtraExtensions, pkix.Extension{
					Id: append(asn1.ObjectIdentifier(nil), extension.Id...), Critical: extension.Critical,
					Value: append([]byte(nil), extension.Value...),
				})
			}
		}
		entries[i] = entry
	}
	return entries, nil
}
