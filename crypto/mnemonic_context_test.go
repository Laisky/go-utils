package crypto

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMnemonicSecurity_ContextAdmission verifies cancellation while queued,
// rejection before KDF work, unencrypted behavior, and reuse after cancellation.
func TestMnemonicSecurity_ContextAdmission(t *testing.T) {
	key, passphrase, data, _ := mnemonicCompatibilityFixture(t)
	phrase := legacyMnemonicEncode(data)
	require.NoError(t, argon2idMemoryBudget.Acquire(context.Background(), argon2idMemoryBudgetKiB))
	held := true
	defer func() {
		if held {
			argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
		}
	}()
	before := argon2idDerivations.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	encoded, err := PrikeyToMnemonicContext(ctx, key, WithMnemonicPassphrase(passphrase))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, encoded)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	decoded, err := MnemonicToPrikeyContext(ctx2, phrase, WithMnemonicPassphrase(passphrase))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, decoded)
	_, err = MnemonicToPrikeyContext(context.Background(), phrase)
	require.ErrorContains(t, err, "no passphrase")
	_, err = mnemonicDecryptDERContext(context.Background(), data[:10], passphrase)
	require.ErrorContains(t, err, "too short")
	plain, err := PrikeyToMnemonicContext(ctx, key)
	require.NoError(t, err, "unencrypted calls do not require KDF admission")
	decoded, err = MnemonicToPrikeyContext(ctx, plain)
	require.NoError(t, err)
	require.Equal(t, key, decoded)
	require.Equal(t, before, argon2idDerivations.Load())
	argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
	held = false
	decoded, err = MnemonicToPrikeyContext(context.Background(), phrase, WithMnemonicPassphrase(passphrase))
	require.NoError(t, err)
	require.Equal(t, key, decoded)
	require.Equal(t, before+1, argon2idDerivations.Load())
}
