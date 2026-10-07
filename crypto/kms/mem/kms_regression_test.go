package mem

import (
	"context"
	"crypto/sha256"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// errKMSOptionFailedForTest is a foreign (standard library) error returned by a
// failing KMSOption in the regression tests below.
var errKMSOptionFailedForTest = stderrors.New("kms option failed for test")

// TestKMSOption_ApplyOptsWrapsOptionError verifies that kmsOption.applyOpts and
// New wrap the error of a failing KMSOption with context instead of returning it
// bare, while errors.Is still matches it. Regression for the documentation-pass
// finding on kmsOption.applyOpts.
func TestKMSOption_ApplyOptsWrapsOptionError(t *testing.T) {
	t.Parallel()

	failing := func(*kmsOption) error { return errKMSOptionFailedForTest }

	opt, err := new(kmsOption).fillDefault().applyOpts(failing)
	require.Nil(t, opt)
	require.ErrorIs(t, err, errKMSOptionFailedForTest)
	require.NotEqual(t, errKMSOptionFailedForTest.Error(), err.Error())

	kms, err := New(map[uint16][]byte{1: []byte("kek-1")}, failing)
	require.Nil(t, kms)
	require.ErrorIs(t, err, errKMSOptionFailedForTest)
}

// TestKMS_DeriveKeyByIDRejectsInvalidLength verifies that KMS.DeriveKeyByID
// returns a contextual error, instead of panicking or returning the HKDF error
// bare, when the requested key length is out of range, and still derives keys
// of valid lengths. Regression for the documentation-pass finding on
// KMS.DeriveKeyByID.
func TestKMS_DeriveKeyByIDRejectsInvalidLength(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	kms, err := New(map[uint16][]byte{1: []byte("kek-1")})
	require.NoError(t, err)

	for _, length := range []int{-1, 0, 255*sha256.Size + 1} {
		require.NotPanics(t, func() {
			dek, err := kms.DeriveKeyByID(ctx, 1, []byte("dek-id"), length)
			require.ErrorContains(t, err, "derive key by kek 1")
			require.Nil(t, dek)
		}, "length %d", length)
	}

	dek, err := kms.DeriveKeyByID(ctx, 1, []byte("dek-id"), 32)
	require.NoError(t, err)
	require.Len(t, dek, 32)
}
