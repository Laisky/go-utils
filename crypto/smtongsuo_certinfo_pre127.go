//go:build !go1.27

package crypto

import "crypto/x509"

// setX509RawSignatureAlgorithm is a no-op before Go 1.27, whose
// x509.Certificate has no RawSignatureAlgorithm field. It always returns nil.
func setX509RawSignatureAlgorithm(_ *x509.Certificate) error {
	return nil
}
