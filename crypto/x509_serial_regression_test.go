package crypto

import (
	"crypto/x509"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// countingSerialGenerator records calls to the legacy serial hook.
type countingSerialGenerator struct {
	calls int
	value int64
}

// SerialNum returns the configured sentinel and records the invocation.
func (g *countingSerialGenerator) SerialNum() int64 {
	g.calls++
	return g.value
}

// TestCertificateSerialGeneratorSelection reproduces an ignored certificate hook.
func TestCertificateSerialGeneratorSelection(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	gen := &countingSerialGenerator{value: 424242}
	der, err := NewX509Cert(key, WithX509CertCommonName("serial"), WithX509CertSerialNumGenerator(gen))
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	require.Equal(t, 1, gen.calls)
	require.Zero(t, cert.SerialNumber.Cmp(big.NewInt(gen.value)))
}

// TestCSRSerialGeneratorControl confirms the CSR hook itself is functional.
func TestCSRSerialGeneratorControl(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	csr, err := NewX509CSR(key, WithX509CSRCommonName("serial"))
	require.NoError(t, err)
	gen := &countingSerialGenerator{value: 424242}
	der, err := NewX509CertByCSR(ca, key, csr, WithX509SerialNumGenerator(gen))
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	require.Equal(t, 1, gen.calls)
	require.Zero(t, cert.SerialNumber.Cmp(big.NewInt(gen.value)))
}

// TestExplicitSerialPrecedenceControl preserves serial precedence in either option order.
func TestExplicitSerialPrecedenceControl(t *testing.T) {
	for _, explicitFirst := range []bool{false, true} {
		gen := &countingSerialGenerator{value: 42}
		opts := []X509CertOption{WithX509CertSerialNumGenerator(gen), WithX509CertSeriaNumber(big.NewInt(99))}
		if explicitFirst {
			opts[0], opts[1] = opts[1], opts[0]
		}
		opts = append(opts, WithX509CertCommonName("serial"))
		_, tpl, err := x509CertOption2Template(opts...)
		require.NoError(t, err)
		require.Zero(t, tpl.SerialNumber.Cmp(big.NewInt(99)))
		require.Zero(t, gen.calls)
	}
}
