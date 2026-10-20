package crypto

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"io"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRandomPositiveSerialNumber verifies rejection sampling, DER bounds, and read failures.
func TestRandomPositiveSerialNumber(t *testing.T) {
	t.Parallel()
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 159), big.NewInt(1))
	input := append(make([]byte, 20), bytes.Repeat([]byte{255}, 20)...)
	serial, err := randomPositiveSerialNumber(bytes.NewReader(input), 159)
	require.NoError(t, err)
	require.Zero(t, serial.Cmp(max))
	der, err := asn1.Marshal(serial)
	require.NoError(t, err)
	var raw asn1.RawValue
	rest, err := asn1.Unmarshal(der, &raw)
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Len(t, raw.Bytes, 20)
	require.Zero(t, raw.Bytes[0]&0x80)
	for _, input := range [][]byte{nil, make([]byte, 19), make([]byte, 20)} {
		got, err := randomPositiveSerialNumber(bytes.NewReader(input), 159)
		require.Error(t, err)
		require.Nil(t, got)
	}
	got, err := randomPositiveSerialNumber(serialErrorReader{}, 159)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.Nil(t, got)
}

// serialErrorReader supplies an explicit entropy failure without changing global readers.
type serialErrorReader struct{}

// Read returns a deterministic entropy failure.
func (serialErrorReader) Read(_ []byte) (int, error) { return 0, io.ErrClosedPipe }

// TestSerialResolution verifies zero default draws for explicit serials/hooks and errors.
func TestSerialResolution(t *testing.T) {
	t.Parallel()
	calls := 0
	generate := func() (*big.Int, error) {
		calls++
		return nil, io.ErrClosedPipe
	}
	o := &signCSROption{}
	require.ErrorIs(t, o.resolveSerialNumber(generate), io.ErrClosedPipe)
	require.Equal(t, 1, calls)
	require.Nil(t, o.serialNumber)
	gen := &countingSerialGenerator{value: 42}
	o = &signCSROption{serialNumGenerator: gen}
	require.NoError(t, o.resolveSerialNumber(generate))
	require.Equal(t, 1, gen.calls)
	require.Equal(t, 1, calls)
	o = &signCSROption{serialNumber: big.NewInt(99), serialNumGenerator: gen}
	require.NoError(t, o.resolveSerialNumber(generate))
	require.Equal(t, 1, gen.calls)
	require.Equal(t, 1, calls)
	o = &signCSROption{}
	require.NoError(t, o.resolveSerialNumber(func() (*big.Int, error) {
		calls++
		return big.NewInt(123), nil
	}))
	require.Equal(t, 2, calls)
	require.Equal(t, int64(123), o.serialNumber.Int64())
	_, err := new(signCSROption).applyOpts(nil, WithX509SerialNumGenerator(nil))
	require.Error(t, err)
	_, err = new(x509V3CertOption).applyOpts(WithX509CertSerialNumGenerator(nil))
	require.Error(t, err)
}

// TestWideSerialRoundTrips verifies old and wide DER serials in direct and CSR issuance.
func TestWideSerialRoundTrips(t *testing.T) {
	t.Parallel()
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	csr, err := NewX509CSR(key, WithX509CSRCommonName("serial"))
	require.NoError(t, err)
	for _, serial := range []*big.Int{big.NewInt(0), big.NewInt(42), new(big.Int).Lsh(big.NewInt(1), 120)} {
		certOpt := WithX509CertSeriaNumber(serial)
		csrOpt := WithX509SignCSRSeriaNumber(serial)
		want := new(big.Int).Set(serial)
		serial.SetInt64(7)
		for range 2 {
			der, err := NewX509Cert(key, WithX509CertCommonName("serial"), certOpt)
			require.NoError(t, err)
			cert, err := x509.ParseCertificate(der)
			require.NoError(t, err)
			require.Zero(t, cert.SerialNumber.Cmp(want))
			require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
			der, err = NewX509CertByCSR(ca, key, csr, csrOpt)
			require.NoError(t, err)
			cert, err = x509.ParseCertificate(der)
			require.NoError(t, err)
			require.Zero(t, cert.SerialNumber.Cmp(want))
			require.NoError(t, cert.CheckSignatureFrom(ca))
		}
	}
}

// TestDefaultSerialIssuance verifies secure defaults survive both public issuance paths.
func TestDefaultSerialIssuance(t *testing.T) {
	t.Parallel()
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	csr, err := NewX509CSR(key, WithX509CSRCommonName("serial"))
	require.NoError(t, err)
	direct, err := NewX509Cert(key, WithX509CertCommonName("serial"))
	require.NoError(t, err)
	issued, err := NewX509CertByCSR(ca, key, csr)
	require.NoError(t, err)
	for _, der := range [][]byte{direct, issued} {
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		require.Positive(t, cert.SerialNumber.Sign())
		require.LessOrEqual(t, cert.SerialNumber.BitLen(), 159)
		require.Greater(t, cert.SerialNumber.BitLen(), 63)
	}
}

// TestSerialOptionCopyIsolation verifies reusable options own their big.Int values.
func TestSerialOptionCopyIsolation(t *testing.T) {
	t.Parallel()
	serial := big.NewInt(123)
	certOpt := WithX509CertSeriaNumber(serial)
	csrOpt := WithX509SignCSRSeriaNumber(serial)
	serial.SetInt64(7)
	for range 2 {
		_, tpl, err := x509CertOption2Template(WithX509CertCommonName("serial"), certOpt)
		require.NoError(t, err)
		require.Equal(t, int64(123), tpl.SerialNumber.Int64())
		tpl.SerialNumber.SetInt64(8)
		opt, err := new(signCSROption).applyOpts(nil, csrOpt)
		require.NoError(t, err)
		require.Equal(t, int64(123), opt.serialNumber.Int64())
		opt.serialNumber.SetInt64(8)
	}
}

// TestCSRExplicitSerialPrecedence verifies hook suppression in both option orders.
func TestCSRExplicitSerialPrecedence(t *testing.T) {
	t.Parallel()
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	csr, err := NewX509CSR(key, WithX509CSRCommonName("serial"))
	require.NoError(t, err)
	for _, explicitFirst := range []bool{false, true} {
		gen := &countingSerialGenerator{value: 42}
		opts := []SignCSROption{WithX509SerialNumGenerator(gen), WithX509SignCSRSeriaNumber(big.NewInt(99))}
		if explicitFirst {
			opts[0], opts[1] = opts[1], opts[0]
		}
		der, err := NewX509CertByCSR(ca, key, csr, opts...)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		require.Zero(t, cert.SerialNumber.Cmp(big.NewInt(99)))
		require.Zero(t, gen.calls)
	}
}

// TestGeneratedSerialConcurrency exercises the secure API concurrently without proving uniqueness.
func TestGeneratedSerialConcurrency(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			serial, err := GenerateX509CertificateSerialNumber()
			require.NoError(t, err)
			require.Positive(t, serial.Sign())
			require.LessOrEqual(t, serial.BitLen(), 159)
		})
	}
	wg.Wait()
}
