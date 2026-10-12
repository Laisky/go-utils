package crypto

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPasswordAdmissionSharedBudget proves both password-ciphertext derivation
// paths wait for the same logical budget as password hashing, without costly KDFs.
func TestPasswordAdmissionSharedBudget(t *testing.T) {
	password := []byte("synthetic-admission-password")
	ct, err := EncryptByPassword(password, []byte("payload"), fastPasswordKDF())
	require.NoError(t, err)
	operations := map[string]func() error{
		"decrypt": func() error { _, err := DecryptByPassword(password, ct); return err },
		"encrypt": func() error { _, err := NewPasswordEncryptor(password, fastPasswordKDF()); return err },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, argon2idMemoryBudget.Acquire(context.Background(), argon2idMemoryBudgetKiB))
			held := true
			defer func() {
				if held {
					argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
				}
			}()
			before := argon2idDerivations.Load()
			done := make(chan error, 1)
			go func() { done <- operation() }()
			select {
			case err := <-done:
				require.NoError(t, err)
				t.Fatal("derivation bypassed the fully occupied shared Argon2 budget")
			case <-time.After(50 * time.Millisecond):
				require.Equal(t, before, argon2idDerivations.Load())
			}
			argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
			held = false
			select {
			case err := <-done:
				require.NoError(t, err)
				require.Equal(t, before+1, argon2idDerivations.Load())
			case <-time.After(5 * time.Second):
				t.Fatal("derivation did not resume after budget release")
			}
		})
	}
}

// TestPasswordAdmissionCancellation verifies all context-aware encryption APIs
// cancel while admission is unavailable and never start a KDF after cancellation.
func TestPasswordAdmissionCancellation(t *testing.T) {
	password := []byte("synthetic-cancel-password")
	ct, err := EncryptByPassword(password, []byte("payload"), fastPasswordKDF())
	require.NoError(t, err)
	operations := map[string]func(context.Context) error{
		"constructor": func(ctx context.Context) error {
			_, err := NewPasswordEncryptorContext(ctx, password, fastPasswordKDF())
			return err
		},
		"encrypt": func(ctx context.Context) error {
			_, err := EncryptByPasswordContext(ctx, password, nil, fastPasswordKDF())
			return err
		},
		"decrypt": func(ctx context.Context) error { _, err := DecryptByPasswordContext(ctx, password, ct); return err },
	}
	require.NoError(t, argon2idMemoryBudget.Acquire(t.Context(), argon2idMemoryBudgetKiB))
	defer argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
	before := argon2idDerivations.Load()
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- operation(ctx) }()
			select {
			case err := <-done:
				cancel()
				t.Fatalf("operation returned before waiting admission was canceled: %v", err)
			case <-time.After(25 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("canceled admission did not return")
			}
			require.Equal(t, before, argon2idDerivations.Load())
		})
	}
}
