package crypto

import (
	"crypto/x509"

	"github.com/Laisky/errors/v2"
	"github.com/emmansun/gmsm/smx509"
)

var (
	// smx509ToX509SignatureAlgorithms maps smx509 signature algorithms that
	// crypto/x509 also knows. Algorithms absent from this map, notably
	// SM2-with-SM3, are reported as x509.UnknownSignatureAlgorithm.
	smx509ToX509SignatureAlgorithms = map[smx509.SignatureAlgorithm]x509.SignatureAlgorithm{
		smx509.MD2WithRSA:       x509.MD2WithRSA,
		smx509.MD5WithRSA:       x509.MD5WithRSA,
		smx509.SHA1WithRSA:      x509.SHA1WithRSA,
		smx509.SHA256WithRSA:    x509.SHA256WithRSA,
		smx509.SHA384WithRSA:    x509.SHA384WithRSA,
		smx509.SHA512WithRSA:    x509.SHA512WithRSA,
		smx509.DSAWithSHA1:      x509.DSAWithSHA1,
		smx509.DSAWithSHA256:    x509.DSAWithSHA256,
		smx509.ECDSAWithSHA1:    x509.ECDSAWithSHA1,
		smx509.ECDSAWithSHA256:  x509.ECDSAWithSHA256,
		smx509.ECDSAWithSHA384:  x509.ECDSAWithSHA384,
		smx509.ECDSAWithSHA512:  x509.ECDSAWithSHA512,
		smx509.SHA256WithRSAPSS: x509.SHA256WithRSAPSS,
		smx509.SHA384WithRSAPSS: x509.SHA384WithRSAPSS,
		smx509.SHA512WithRSAPSS: x509.SHA512WithRSAPSS,
		smx509.PureEd25519:      x509.PureEd25519,
	}

	// smx509ToX509ExtKeyUsages maps every smx509 extended key usage to the
	// crypto/x509 value with the same OID.
	smx509ToX509ExtKeyUsages = map[smx509.ExtKeyUsage]x509.ExtKeyUsage{
		smx509.ExtKeyUsageAny:                            x509.ExtKeyUsageAny,
		smx509.ExtKeyUsageServerAuth:                     x509.ExtKeyUsageServerAuth,
		smx509.ExtKeyUsageClientAuth:                     x509.ExtKeyUsageClientAuth,
		smx509.ExtKeyUsageCodeSigning:                    x509.ExtKeyUsageCodeSigning,
		smx509.ExtKeyUsageEmailProtection:                x509.ExtKeyUsageEmailProtection,
		smx509.ExtKeyUsageIPSECEndSystem:                 x509.ExtKeyUsageIPSECEndSystem,
		smx509.ExtKeyUsageIPSECTunnel:                    x509.ExtKeyUsageIPSECTunnel,
		smx509.ExtKeyUsageIPSECUser:                      x509.ExtKeyUsageIPSECUser,
		smx509.ExtKeyUsageTimeStamping:                   x509.ExtKeyUsageTimeStamping,
		smx509.ExtKeyUsageOCSPSigning:                    x509.ExtKeyUsageOCSPSigning,
		smx509.ExtKeyUsageMicrosoftServerGatedCrypto:     x509.ExtKeyUsageMicrosoftServerGatedCrypto,
		smx509.ExtKeyUsageNetscapeServerGatedCrypto:      x509.ExtKeyUsageNetscapeServerGatedCrypto,
		smx509.ExtKeyUsageMicrosoftCommercialCodeSigning: x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
		smx509.ExtKeyUsageMicrosoftKernelCodeSigning:     x509.ExtKeyUsageMicrosoftKernelCodeSigning,
	}
)

// smx509OIDToX509 converts an smx509 OID into the crypto/x509 OID with the
// same DER encoding. It returns an error if the encoding is rejected.
func smx509OIDToX509(oid smx509.OID) (x509.OID, error) {
	der, err := oid.MarshalBinary()
	if err != nil {
		return x509.OID{}, errors.Wrap(err, "marshal oid")
	}

	var out x509.OID
	if err = out.UnmarshalBinary(der); err != nil {
		return x509.OID{}, errors.Wrap(err, "unmarshal oid")
	}

	return out, nil
}

// smx509OIDsToX509 converts a list of smx509 OIDs, preserving order. It
// returns nil for an empty input and an error if any OID cannot be converted.
func smx509OIDsToX509(oids []smx509.OID) ([]x509.OID, error) {
	if len(oids) == 0 {
		return nil, nil
	}

	out := make([]x509.OID, 0, len(oids))
	for _, oid := range oids {
		converted, err := smx509OIDToX509(oid)
		if err != nil {
			return nil, errors.Wrapf(err, "convert oid %s", oid.String())
		}
		out = append(out, converted)
	}

	return out, nil
}

// smx509PolicyMappingsToX509 converts policy mappings, preserving order. It
// returns nil for an empty input and an error if any OID cannot be converted.
func smx509PolicyMappingsToX509(mappings []smx509.PolicyMapping) ([]x509.PolicyMapping, error) {
	if len(mappings) == 0 {
		return nil, nil
	}

	out := make([]x509.PolicyMapping, 0, len(mappings))
	for _, m := range mappings {
		issuer, err := smx509OIDToX509(m.IssuerDomainPolicy)
		if err != nil {
			return nil, errors.Wrap(err, "convert issuer domain policy")
		}
		subject, err := smx509OIDToX509(m.SubjectDomainPolicy)
		if err != nil {
			return nil, errors.Wrap(err, "convert subject domain policy")
		}
		out = append(out, x509.PolicyMapping{IssuerDomainPolicy: issuer, SubjectDomainPolicy: subject})
	}

	return out, nil
}

// smx509ExtKeyUsagesToX509 converts parsed extended key usages, preserving
// order. It returns an error for any value without a crypto/x509 equivalent so
// that no usage is silently dropped.
func smx509ExtKeyUsagesToX509(usages []smx509.ExtKeyUsage) ([]x509.ExtKeyUsage, error) {
	if len(usages) == 0 {
		return nil, nil
	}

	out := make([]x509.ExtKeyUsage, 0, len(usages))
	for _, usage := range usages {
		converted, ok := smx509ToX509ExtKeyUsages[usage]
		if !ok {
			return nil, errors.Errorf("unsupported ext key usage %d", usage)
		}
		out = append(out, converted)
	}

	return out, nil
}

// smx509CertificateToX509 copies every parsed field of an smx509 certificate
// into a crypto/x509 certificate. The public key is deliberately not copied:
// PublicKeyAlgorithm is x509.UnknownPublicKeyAlgorithm and PublicKey is nil,
// because crypto/x509 cannot represent SM2 keys and labeling them ECDSA would
// be misleading. Signature algorithms unknown to crypto/x509, such as
// SM2-with-SM3, become x509.UnknownSignatureAlgorithm. It returns an error if
// an OID or extended key usage cannot be converted exactly.
func smx509CertificateToX509(c *smx509.Certificate) (*x509.Certificate, error) {
	policies, err := smx509OIDsToX509(c.Policies)
	if err != nil {
		return nil, errors.Wrap(err, "convert policies")
	}
	mappings, err := smx509PolicyMappingsToX509(c.PolicyMappings)
	if err != nil {
		return nil, errors.Wrap(err, "convert policy mappings")
	}
	extKeyUsages, err := smx509ExtKeyUsagesToX509(c.ExtKeyUsage)
	if err != nil {
		return nil, errors.Wrap(err, "convert ext key usages")
	}

	cert := &x509.Certificate{
		Raw:                     c.Raw,
		RawTBSCertificate:       c.RawTBSCertificate,
		RawSubjectPublicKeyInfo: c.RawSubjectPublicKeyInfo,
		RawSubject:              c.RawSubject,
		RawIssuer:               c.RawIssuer,

		Signature:          c.Signature,
		SignatureAlgorithm: smx509ToX509SignatureAlgorithms[c.SignatureAlgorithm],

		PublicKeyAlgorithm: x509.UnknownPublicKeyAlgorithm,
		PublicKey:          nil,

		Version:      c.Version,
		SerialNumber: c.SerialNumber,
		Issuer:       c.Issuer,
		Subject:      c.Subject,
		NotBefore:    c.NotBefore,
		NotAfter:     c.NotAfter,
		KeyUsage:     x509.KeyUsage(c.KeyUsage),

		Extensions:                  c.Extensions,
		UnhandledCriticalExtensions: c.UnhandledCriticalExtensions,

		ExtKeyUsage:        extKeyUsages,
		UnknownExtKeyUsage: c.UnknownExtKeyUsage,

		BasicConstraintsValid: c.BasicConstraintsValid,
		IsCA:                  c.IsCA,
		MaxPathLen:            c.MaxPathLen,
		MaxPathLenZero:        c.MaxPathLenZero,

		SubjectKeyId:   c.SubjectKeyId,
		AuthorityKeyId: c.AuthorityKeyId,

		OCSPServer:            c.OCSPServer,
		IssuingCertificateURL: c.IssuingCertificateURL,

		DNSNames:       c.DNSNames,
		EmailAddresses: c.EmailAddresses,
		IPAddresses:    c.IPAddresses,
		URIs:           c.URIs,

		PermittedDNSDomainsCritical: c.PermittedDNSDomainsCritical,
		PermittedDNSDomains:         c.PermittedDNSDomains,
		ExcludedDNSDomains:          c.ExcludedDNSDomains,
		PermittedIPRanges:           c.PermittedIPRanges,
		ExcludedIPRanges:            c.ExcludedIPRanges,
		PermittedEmailAddresses:     c.PermittedEmailAddresses,
		ExcludedEmailAddresses:      c.ExcludedEmailAddresses,
		PermittedURIDomains:         c.PermittedURIDomains,
		ExcludedURIDomains:          c.ExcludedURIDomains,

		CRLDistributionPoints: c.CRLDistributionPoints,

		PolicyIdentifiers: c.PolicyIdentifiers,
		Policies:          policies,

		InhibitAnyPolicy:          c.InhibitAnyPolicy,
		InhibitAnyPolicyZero:      c.InhibitAnyPolicyZero,
		InhibitPolicyMapping:      c.InhibitPolicyMapping,
		InhibitPolicyMappingZero:  c.InhibitPolicyMappingZero,
		RequireExplicitPolicy:     c.RequireExplicitPolicy,
		RequireExplicitPolicyZero: c.RequireExplicitPolicyZero,
		PolicyMappings:            mappings,
	}
	if err = setX509RawSignatureAlgorithm(cert); err != nil {
		return nil, errors.Wrap(err, "set raw signature algorithm")
	}

	return cert, nil
}
