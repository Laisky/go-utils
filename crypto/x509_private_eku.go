package crypto

import (
	"encoding/asn1"

	"github.com/Laisky/errors/v2"
)

// copyX509UnknownExtKeyUsage validates and copies purposes without aliasing their arcs.
// It returns nil for no purposes and rejects an invalid OID before returning any copy.
func copyX509UnknownExtKeyUsage(purposes []asn1.ObjectIdentifier) ([]asn1.ObjectIdentifier, error) {
	if len(purposes) == 0 {
		return nil, nil
	}

	copied := make([]asn1.ObjectIdentifier, len(purposes))
	for i, purpose := range purposes {
		der, err := asn1.Marshal(purpose)
		if err != nil {
			return nil, errors.Wrapf(err, "encode extended key usage OID %d", i)
		}
		var parsed asn1.ObjectIdentifier
		rest, err := asn1.Unmarshal(der, &parsed)
		if err != nil {
			return nil, errors.Wrapf(err, "parse extended key usage OID %d", i)
		}
		if len(rest) != 0 || !parsed.Equal(purpose) {
			return nil, errors.Errorf("extended key usage OID %d cannot be encoded exactly", i)
		}
		copied[i] = parsed
	}
	return copied, nil
}

// WithX509CertUnknownExtKeyUsage adds OID purposes to a Go-issued certificate.
// It copies purposes when created and on each application, and returns an option
// that rejects malformed or empty OIDs atomically. No arguments is a no-op.
// Raw EKU ExtraExtensions retain precedence over these and the enum purposes.
// This option is unsupported by Tongsuo. It does not change chain authorization.
func WithX509CertUnknownExtKeyUsage(purposes ...asn1.ObjectIdentifier) X509CertOption {
	captured, captureErr := copyX509UnknownExtKeyUsage(purposes)
	return func(o *x509V3CertOption) error {
		if captureErr != nil {
			return errors.Wrap(captureErr, "invalid extended key usage OIDs")
		}
		copied, err := copyX509UnknownExtKeyUsage(captured)
		if err != nil {
			return errors.Wrap(err, "copy extended key usage OIDs")
		}
		o.unknownExtKeyUsage = append(o.unknownExtKeyUsage, copied...)
		return nil
	}
}

// WithX509SignCSRUnknownExtKeyUsage adds authorized OID purposes when Go signs a CSR.
// It copies purposes when created and on each application, and returns an option
// that rejects malformed or empty OIDs atomically. No arguments is a no-op.
// Requested CSR Extensions are not automatically authorized for issuance; callers
// must select permitted purposes. Raw EKU ExtraExtensions retain precedence.
// This option is unsupported by Tongsuo. It does not change chain authorization.
func WithX509SignCSRUnknownExtKeyUsage(purposes ...asn1.ObjectIdentifier) SignCSROption {
	captured, captureErr := copyX509UnknownExtKeyUsage(purposes)
	return func(o *signCSROption) error {
		if captureErr != nil {
			return errors.Wrap(captureErr, "invalid extended key usage OIDs")
		}
		copied, err := copyX509UnknownExtKeyUsage(captured)
		if err != nil {
			return errors.Wrap(err, "copy extended key usage OIDs")
		}
		o.unknownExtKeyUsage = append(o.unknownExtKeyUsage, copied...)
		return nil
	}
}
