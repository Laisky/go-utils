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

// offCurveECDSAKeys derives invalid public keys from a valid key on the same
// NIST curve. It takes the valid key and returns named keys whose coordinates
// are off the curve, out of the field range, negative, oversized, or the
// encoding of the point at infinity.
func offCurveECDSAKeys(key *ecdsa.PublicKey) map[string]*ecdsa.PublicKey {
	params := key.Curve.Params()
	one := big.NewInt(1)
	derive := func(x, y *big.Int) *ecdsa.PublicKey {
		return &ecdsa.PublicKey{Curve: key.Curve, X: x, Y: y}
	}
	return map[string]*ecdsa.PublicKey{
		"y+1":            derive(key.X, new(big.Int).Add(key.Y, one)),
		"x+1":            derive(new(big.Int).Add(key.X, one), key.Y),
		"infinity":       derive(big.NewInt(0), big.NewInt(0)),
		"x+p congruent":  derive(new(big.Int).Add(key.X, params.P), key.Y),
		"y+p congruent":  derive(key.X, new(big.Int).Add(key.Y, params.P)),
		"y=p":            derive(key.X, new(big.Int).Set(params.P)),
		"negative x":     derive(new(big.Int).Neg(key.X), key.Y),
		"negative y":     derive(key.X, new(big.Int).Neg(key.Y)),
		"oversized x":    derive(new(big.Int).Lsh(one, uint(params.BitSize+8)), key.Y),
		"generator y+1":  derive(params.Gx, new(big.Int).Add(params.Gy, one)),
		"swapped coords": derive(key.Y, key.X),
	}
}

// TestECDSAVerificationRejectsOffCurveKeys verifies that public-key validation
// rejects every off-curve or out-of-range point on all supported NIST curves,
// accepts valid points (including the negated point), and that each public
// verification API fails closed for such keys. It guards the replacement of
// the deprecated elliptic.Curve.IsOnCurve call (staticcheck SA1019).
func TestECDSAVerificationRejectsOffCurveKeys(t *testing.T) {
	t.Parallel()
	message := []byte("synthetic off-curve fixture")
	digest := sha256.Sum256(message)
	for _, curve := range []elliptic.Curve{elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		t.Run(curve.Params().Name, func(t *testing.T) {
			t.Parallel()
			key, err := ecdsa.GenerateKey(curve, rand.Reader)
			require.NoError(t, err)
			r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
			require.NoError(t, err)

			require.True(t, validECDSAVerificationInputs(&key.PublicKey, r, s))
			negated := &ecdsa.PublicKey{Curve: curve, X: key.X, Y: new(big.Int).Sub(curve.Params().P, key.Y)}
			require.True(t, validECDSAVerificationInputs(negated, r, s), "the negated point is on the curve")
			generator := &ecdsa.PublicKey{Curve: curve, X: curve.Params().Gx, Y: curve.Params().Gy}
			require.True(t, validECDSAVerificationInputs(generator, r, s))

			for name, bad := range offCurveECDSAKeys(&key.PublicKey) {
				require.NotPanics(t, func() {
					require.False(t, validECDSAVerificationInputs(bad, r, s), name)
					require.False(t, VerifyByECDSAWithSHA256(bad, message, r, s), name)
					ok, err := VerifyByECDSAWithSHA256AndBase64(bad, message, EncodeES256SignByBase64(r, s))
					require.Error(t, err, name)
					require.False(t, ok, name)
					ok, err = VerifyReaderByECDSAWithSHA256(bad, bytes.NewReader(message), r, s)
					require.Error(t, err, name)
					require.False(t, ok, name)
				}, name)
			}
		})
	}
}
