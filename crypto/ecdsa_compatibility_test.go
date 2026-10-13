package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// compatibilityCurve wraps a standard curve without changing its mathematical parameters.
type compatibilityCurve struct{ elliptic.Curve }

// TestECDSACompatibilityStandardWrappers accepts standard-curve wrappers in every verification entrypoint.
func TestECDSACompatibilityStandardWrappers(t *testing.T) {
	message := []byte("standard curve wrapper compatibility")
	digest := sha256.Sum256(message)
	for _, curve := range []elliptic.Curve{elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		t.Run(curve.Params().Name, func(t *testing.T) {
			key, err := ecdsa.GenerateKey(curve, rand.Reader)
			require.NoError(t, err)
			r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
			require.NoError(t, err)
			wrapped := key.PublicKey
			wrapped.Curve = compatibilityCurve{curve}
			require.True(t, ecdsa.Verify(&wrapped, digest[:], r, s), "historical standard-library control")
			require.True(t, VerifyByECDSAWithSHA256(&wrapped, message, r, s))
			ok, err := VerifyByECDSAWithSHA256AndBase64(&wrapped, message, EncodeES256SignByBase64(r, s))
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = VerifyReaderByECDSAWithSHA256(&wrapped, bytes.NewReader(message), r, s)
			require.NoError(t, err)
			require.True(t, ok)
			require.IsType(t, compatibilityCurve{}, wrapped.Curve, "verification must not mutate the caller's key")
		})
	}
}

// hostileCompatibilityCurve declares parameters but panics if its arithmetic is used.
type hostileCompatibilityCurve struct {
	elliptic.Curve
	params *elliptic.CurveParams
}

// Params returns the declared parameters without providing a trusted arithmetic implementation.
func (c hostileCompatibilityCurve) Params() *elliptic.CurveParams { return c.params }

// IsOnCurve panics to prove verification uses only standard-library curve arithmetic.
func (c hostileCompatibilityCurve) IsOnCurve(_, _ *big.Int) bool { panic("untrusted curve arithmetic") }

// ScalarMult panics to prove verification never dispatches scalar multiplication to a wrapper.
func (c hostileCompatibilityCurve) ScalarMult(_, _ *big.Int, _ []byte) (*big.Int, *big.Int) {
	panic("untrusted curve arithmetic")
}

// TestECDSACompatibilityWrapperValidation rejects forged parameters and invalid points without wrapper arithmetic.
func TestECDSACompatibilityWrapperValidation(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	message := []byte("hostile standard-curve wrapper")
	r, s, err := SignByECDSAWithSHA256(key, message)
	require.NoError(t, err)
	params := *elliptic.P256().Params()
	params.Name = "compatible wrapper metadata"
	wrapped := key.PublicKey
	wrapped.Curve = hostileCompatibilityCurve{params: &params}
	require.True(t, VerifyByECDSAWithSHA256(&wrapped, message, r, s))
	require.False(t, VerifyByECDSAWithSHA256(&wrapped, []byte("changed"), r, s))
	bad := params
	bad.N = new(big.Int).Sub(params.N, big.NewInt(1))
	wrapped.Curve = hostileCompatibilityCurve{params: &bad}
	require.False(t, VerifyByECDSAWithSHA256(&wrapped, message, r, s))
	wrapped.Curve = hostileCompatibilityCurve{}
	require.False(t, VerifyByECDSAWithSHA256(&wrapped, message, r, s))
	var nilParams *elliptic.CurveParams
	wrapped.Curve = nilParams
	require.NotPanics(t, func() { require.False(t, VerifyByECDSAWithSHA256(&wrapped, message, r, s)) })
	wrapped.Curve = hostileCompatibilityCurve{params: &params}
	wrapped.X = big.NewInt(0)
	wrapped.Y = big.NewInt(0)
	require.False(t, VerifyByECDSAWithSHA256(&wrapped, message, r, s))
}
