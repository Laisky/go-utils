package crypto

import (
	"crypto/rand"
	"crypto/x509"
	"io"
	"math/big"
	"time"

	"github.com/Laisky/errors/v2"
)

// GenerateX509CertificateSerialNumber returns a uniformly random positive serial
// smaller than 2^159, or an entropy error. Its DER INTEGER content fits 20 octets.
// Randomness does not guarantee issuer uniqueness; issuers requiring a hard
// uniqueness guarantee must reserve serials in durable storage.
func GenerateX509CertificateSerialNumber() (*big.Int, error) {
	return randomPositiveSerialNumber(rand.Reader, 159)
}

// randomPositiveSerialNumber reads uniformly random nonzero bits from reader.
// It retries zero and returns read failures without a predictable fallback.
func randomPositiveSerialNumber(reader io.Reader, bits uint) (*big.Int, error) {
	if bits == 0 || bits > 159 {
		return nil, errors.New("serial bit width must be between 1 and 159")
	}
	buf := make([]byte, (bits+7)/8)
	excessBits := uint(len(buf)*8) - bits
	for {
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, errors.Wrap(err, "read certificate serial entropy")
		}
		buf[0] &= byte(0xff >> excessBits)
		serial := new(big.Int).SetBytes(buf)
		if serial.Sign() > 0 {
			return serial, nil
		}
	}
}

// fillSigningDefaults initializes signing defaults without consuming entropy.
func (o *signCSROption) fillSigningDefaults() {
	o.notBefore = time.Now().UTC()
	o.notAfter = o.notBefore.Add(7 * 24 * time.Hour)
	o.keyUsage |= x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
}

// resolveSerialNumber selects an explicit serial, legacy hook, or default exactly
// once after all options. generate is private so tests can inject entropy errors.
func (o *signCSROption) resolveSerialNumber(generate func() (*big.Int, error)) error {
	if o.serialNumber != nil {
		o.serialNumber = new(big.Int).Set(o.serialNumber)
		return nil
	}
	if o.serialNumGenerator != nil {
		o.serialNumber = big.NewInt(o.serialNumGenerator.SerialNum())
		return nil
	}
	serial, err := generate()
	if err != nil {
		return errors.Wrap(err, "generate certificate serial number")
	}
	o.serialNumber = serial
	return nil
}
