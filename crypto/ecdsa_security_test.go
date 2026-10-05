package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestSecurity53MalformedBase64 checks malformed delimiters, alphabets, and zero components.
func TestSecurity53MalformedBase64(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	for _, input := range []string{"", "AAAA", "a.b.c", ".AQ==", "AQ==.", "AQ==.AQ==.", "@@.AA==", "AA==.AQ==", "AQ==.AA==", "AQ==.\nAQ==", "AR==.AQ=="} {
		t.Run(input, func(t *testing.T) {
			r, s, err := DecodeES256SignByBase64(input)
			require.Error(t, err)
			require.Nil(t, r)
			require.Nil(t, s)
			require.LessOrEqual(t, len(err.Error()), 160)
			require.NotPanics(t, func() {
				ok, err := VerifyByECDSAWithSHA256AndBase64(&key.PublicKey, []byte("message"), input)
				require.Error(t, err)
				require.False(t, ok)
			})
		})
	}
}

// TestSecurity57BoundedParsing checks early rejection and bounded diagnostics.
func TestSecurity57BoundedParsing(t *testing.T) {
	for _, input := range []string{strings.Repeat("a", 4096) + "." + strings.Repeat("a", 4096), strings.Repeat(".", 65536), "-1.1", "+1.1", "0.1", "1.0", "1..2"} {
		r, s, err := DecodeES256SignByHex(input)
		require.Error(t, err)
		require.Nil(t, r)
		require.Nil(t, s)
		require.LessOrEqual(t, len(err.Error()), 160)
	}
	tooLong := strings.Repeat("A", 65536)
	allocations := testing.AllocsPerRun(100, func() { _, _, _ = DecodeES256SignByBase64(tooLong) })
	// A fixed-size rejection should not allocate per delimiter/input byte.
	require.LessOrEqual(t, allocations, float64(16))
}

// TestSecurity53VerificationInputs checks direct APIs with nil and out-of-range integers.
func TestSecurity53VerificationInputs(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	one := big.NewInt(1)
	for _, component := range []*big.Int{nil, big.NewInt(0), big.NewInt(-1), new(big.Int).Set(key.Params().N), new(big.Int).Lsh(one, 1024)} {
		require.NotPanics(t, func() { require.False(t, VerifyByECDSAWithSHA256(&key.PublicKey, nil, component, one)) })
		require.NotPanics(t, func() { require.False(t, VerifyByECDSAWithSHA256(&key.PublicKey, nil, one, component)) })
		require.NotPanics(t, func() {
			ok, err := VerifyReaderByECDSAWithSHA256(&key.PublicKey, bytes.NewReader(nil), component, one)
			require.Error(t, err)
			require.False(t, ok)
		})
	}
	for _, pub := range []*ecdsa.PublicKey{nil, {}, {Curve: elliptic.P256()}, {Curve: elliptic.P256(), X: one, Y: one}} {
		require.NotPanics(t, func() { require.False(t, VerifyByECDSAWithSHA256(pub, nil, one, one)) })
	}
}

// TestSecurity53NISTCompatibility verifies encodings across every supported curve.
func TestSecurity53NISTCompatibility(t *testing.T) {
	message := []byte("synthetic bounded signature fixture")
	digest := sha256.Sum256(message)
	for _, curve := range []elliptic.Curve{elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		t.Run(curve.Params().Name, func(t *testing.T) {
			key, err := ecdsa.GenerateKey(curve, rand.Reader)
			require.NoError(t, err)
			r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
			require.NoError(t, err)
			for _, decode := range []struct {
				encoded string
				f       func(string) (*big.Int, *big.Int, error)
			}{
				{EncodeES256SignByHex(r, s), DecodeES256SignByHex}, {EncodeES256SignByBase64(r, s), DecodeES256SignByBase64},
			} {
				rr, ss, err := decode.f(decode.encoded)
				require.NoError(t, err)
				require.Equal(t, r, rr)
				require.Equal(t, s, ss)
			}
			require.True(t, VerifyByECDSAWithSHA256(&key.PublicKey, message, r, s))
			require.False(t, VerifyByECDSAWithSHA256(&key.PublicKey, []byte("changed"), r, s))
			ok, err := VerifyByECDSAWithSHA256AndBase64(&key.PublicKey, message, EncodeES256SignByBase64(r, s))
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = VerifyReaderByECDSAWithSHA256(&key.PublicKey, bytes.NewReader(message), r, s)
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = VerifyReaderByECDSAWithSHA256(&key.PublicKey, nil, r, s)
			require.False(t, ok)
			require.Error(t, err)
			synthetic := errors.New("synthetic reader failure")
			ok, err = VerifyReaderByECDSAWithSHA256(&key.PublicKey, iotest.ErrReader(synthetic), r, s)
			require.False(t, ok)
			require.ErrorIs(t, err, synthetic)
		})
	}
}

// FuzzSecurity53SignatureDecoding checks panic freedom under a bounded fuzz input.
func FuzzSecurity53SignatureDecoding(f *testing.F) {
	for _, input := range []string{"", "AAAA", "AQ==.AQ==", "a.b.c", "1.1"} {
		f.Add(input)
	}
	x, y := elliptic.P256().ScalarBaseMult([]byte{1})
	key := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}
		_, _, _ = DecodeES256SignByHex(input)
		_, _, _ = DecodeES256SignByBase64(input)
		_, _ = VerifyByECDSAWithSHA256AndBase64(key, nil, input)
	})
}
