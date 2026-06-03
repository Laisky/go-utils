// Package mem is a multi-key KMS in pure memory
package mem

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
	gcounter "github.com/Laisky/go-utils/v6/counter"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	gkms "github.com/Laisky/go-utils/v6/crypto/kms"
)

func TestKMS_Decrypt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mk, err := gcrypto.Salt(128)
	require.NoError(t, err)

	kms, err := New(map[uint16][]byte{
		1: mk,
	})
	require.NoError(t, err)

	gt := gutils.NewGoroutineTest(t, cancel)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			ks, err := kms.Keks(ctx)
			require.NoError(gt, err)
			require.Equal(gt, mk, ks[1])
		}
	}()

	go func() {
		counter := gcounter.NewCounterFromN(1)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			mk, err := gcrypto.Salt(128)
			require.NoError(gt, err)
			kms.AddKek(ctx, uint16(counter.Count()), mk)
			time.Sleep(time.Millisecond)
		}
	}()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 1; i++ {
		time.Sleep(time.Millisecond)
		wg.Add(1)
		go func() {
			defer wg.Done()

			plaintext, err := gcrypto.Salt(1024 + rand.Intn(1024))
			require.NoError(gt, err)
			ei, err := kms.Encrypt(ctx, plaintext, []byte("laisky"))
			require.NoError(gt, err)

			t.Run("encrypt by id", func(t *testing.T) {
				gotcipher, err := kms.EncryptByID(ctx, plaintext, []byte("laisky"), ei.KekID, ei.DekID)
				require.NoError(gt, err)
				require.NotEqual(gt, ei.Ciphertext, gotcipher)

				gotplain, err := kms.Decrypt(ctx, &gkms.EncryptedData{
					Version:    ei.Version,
					KekID:      ei.KekID,
					DekID:      ei.DekID,
					Ciphertext: ei.Ciphertext,
				}, []byte("laisky"))
				require.NoError(gt, err)
				require.Equal(gt, plaintext, gotplain)
			})

			t.Run("decrypt", func(t *testing.T) {
				gotplain, err := kms.Decrypt(ctx, ei, []byte("laisky"))
				require.NoError(gt, err)
				require.Equal(gt, plaintext, gotplain)
			})

			t.Run("decrypt with wrong add", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, ei, []byte("laisky123"))
				require.ErrorContains(gt, err, "message authentication failed")
			})

			t.Run("decrypt with nonexists dek id", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, &gkms.EncryptedData{
					Version:    ei.Version,
					KekID:      0,
					DekID:      ei.DekID,
					Ciphertext: ei.Ciphertext,
				}, []byte("laisky123"))
				require.ErrorContains(gt, err, "kek 0 not found")
			})

			t.Run("decrypt with wrong dek id", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, &gkms.EncryptedData{
					Version:    ei.Version,
					KekID:      3,
					DekID:      ei.DekID,
					Ciphertext: ei.Ciphertext,
				}, []byte("laisky123"))
				require.ErrorContains(gt, err, "cipher: message authentication failed")
			})

			t.Run("decrypt with wrong dek key id", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, &gkms.EncryptedData{
					Version:    ei.Version,
					KekID:      ei.KekID,
					DekID:      []byte("123"),
					Ciphertext: ei.Ciphertext,
				}, []byte("laisky123"))
				require.ErrorContains(gt, err, "message authentication failed")
			})
		}()
	}

	wg.Wait()
}

// TestKMS_KekDefensiveCopy is a regression test ensuring Kek and Keks return
// defensive copies. A caller mutating the returned slice must not corrupt the
// internal in-memory KEK material.
func TestKMS_KekDefensiveCopy(t *testing.T) {
	ctx := context.Background()

	mk, err := gcrypto.Salt(128)
	require.NoError(t, err)
	original := make([]byte, len(mk))
	copy(original, mk)

	kms, err := New(map[uint16][]byte{1: mk})
	require.NoError(t, err)

	t.Run("Kek", func(t *testing.T) {
		_, kek, err := kms.Kek(ctx)
		require.NoError(t, err)
		require.Equal(t, original, kek)

		// mutate the returned slice
		for i := range kek {
			kek[i] ^= 0xff
		}

		// fetch again, internal copy must be unchanged
		_, kek2, err := kms.Kek(ctx)
		require.NoError(t, err)
		require.Equal(t, original, kek2, "internal KEK must not be mutated via returned slice")
	})

	t.Run("Keks", func(t *testing.T) {
		keks, err := kms.Keks(ctx)
		require.NoError(t, err)
		require.Equal(t, original, keks[1])

		// mutate the returned slice
		for i := range keks[1] {
			keks[1][i] ^= 0xff
		}

		// fetch again, internal copy must be unchanged
		keks2, err := kms.Keks(ctx)
		require.NoError(t, err)
		require.Equal(t, original, keks2[1], "internal KEK must not be mutated via returned map entry")
	})
}
