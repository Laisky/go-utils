package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTongsuoVerifyBySm2Sm3ReportsVerificationFailure verifies, with the real
// tongsuo binary, that VerifyBySm2Sm3 accepts a valid SM2 signature and that a
// tampered signature or message yields an error matching
// ErrSm2SignatureVerification, so callers can tell a bad signature apart from
// an operational failure. Since stdout is no longer folded into errors, the
// "Verification failure" text tongsuo prints is not otherwise visible.
func TestTongsuoVerifyBySm2Sm3ReportsVerificationFailure(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)
	pubkeyPem, err := ins.Prikey2Pubkey(ctx, prikeyPem)
	require.NoError(t, err)

	msg := []byte("sm2 signed message")
	sig, err := ins.SignBySm2Sm3(ctx, prikeyPem, msg)
	require.NoError(t, err)
	require.NoError(t, ins.VerifyBySm2Sm3(ctx, pubkeyPem, sig, msg))

	err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, sig, []byte("tampered message"))
	require.ErrorIs(t, err, ErrSm2SignatureVerification)

	// An operational failure (unparsable public key) must not be reported as a
	// signature mismatch.
	err = ins.VerifyBySm2Sm3(ctx, []byte("not a pem public key"), sig, msg)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrSm2SignatureVerification)
}
