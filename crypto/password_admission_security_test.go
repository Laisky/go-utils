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
