package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
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
