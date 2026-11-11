package crypto

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMnemonicSecurity_CancelAdmittedKDF cancels each real encrypted operation
// after admission and verifies that cancellation suppresses successful output.
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
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			before := argon2idDerivations.Load()
			type result struct {
				output bool
				err    error
			}
			done := make(chan result, 1)
			go func() { output, err := operation.run(ctx); done <- result{output, err} }()
			deadline := time.Now().Add(10 * time.Second)
			for argon2idDerivations.Load() == before {
				select {
				case <-done:
					t.Fatal("operation finished before admission was observed")
				default:
				}
				if time.Now().After(deadline) {
					t.Fatal("KDF was not admitted")
				}
				runtime.Gosched()
			}
			// Exclude a vacuous test where cancellation happens after completion.
			select {
			case <-done:
				t.Fatal("operation finished before cancellation")
			default:
			}
			cancel()
			select {
			case got := <-done:
				require.ErrorIs(t, got.err, context.Canceled)
				require.False(t, got.output, "canceled operation must not return key material")
			case <-time.After(30 * time.Second):
				t.Fatal("admitted KDF did not finish")
			}
			require.True(t, argon2idMemoryBudget.TryAcquire(argon2idMemoryBudgetKiB), "cancellation must release admission")
			argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
		})
	}
}
