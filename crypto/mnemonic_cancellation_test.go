package crypto

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMnemonicSecurity_CancelAdmittedKDF cancels each real encrypted operation
// at the post-derivation context check and verifies cancellation suppresses output.
func TestMnemonicSecurity_CancelAdmittedKDF(t *testing.T) {
	key, passphrase, data, _ := mnemonicCompatibilityFixture(t)
	phrase := legacyMnemonicEncode(data)
	for _, operation := range []struct {
		name string
		run  func(context.Context) (bool, error)
	}{
		{"export", func(ctx context.Context) (bool, error) {
			result, err := PrikeyToMnemonicContext(ctx, key, WithMnemonicPassphrase(passphrase))
			return result != "", err
		}},
		{"import", func(ctx context.Context) (bool, error) {
			result, err := MnemonicToPrikeyContext(ctx, phrase, WithMnemonicPassphrase(passphrase))
			return result != nil, err
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			before := argon2idDerivations.Load()
			ctx := cancelAfterDerivationContext{
				Context: base,
				cancel:  cancel,
				before:  before,
			}
			output, err := operation.run(ctx)
			require.Equal(t, before+1, argon2idDerivations.Load(), "the real KDF must run")
			require.ErrorIs(t, base.Err(), context.Canceled, "post-derivation check must be reached")
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, output, "canceled operation must not return key material")
			require.True(t, argon2idMemoryBudget.TryAcquire(argon2idMemoryBudgetKiB), "cancellation must release admission")
			argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
		})
	}
}

// cancelAfterDerivationContext cancels its parent at the first context check
// after a real KDF has started. These tests are serial: parallel tests cannot
// change the derivation counter while this context is in use. Unlike polling
// from another goroutine, cancellation and observation occur in the same call,
// so scheduler delays cannot move cancellation past the check under test.
// No production hook, fake KDF, sleep, or mutable global callback is needed.
type cancelAfterDerivationContext struct {
	context.Context
	cancel context.CancelFunc
	before uint64
}

// Err cancels after derivation admission and returns the parent context error.
// Admission checks see the original uncanceled context; the next Err call is
// the real helper's post-Argon2 cancellation check.
func (c cancelAfterDerivationContext) Err() error {
	if argon2idDerivations.Load() > c.before {
		c.cancel()
	}
	return c.Context.Err()
}
