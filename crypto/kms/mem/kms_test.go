// Package mem is a multi-key KMS in pure memory
package mem

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v4"
	gcounter "github.com/Laisky/go-utils/v4/counter"
	gcrypto "github.com/Laisky/go-utils/v4/crypto"
	"github.com/stretchr/testify/require"
)

func TestKMS_Decrypt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mk, err := gcrypto.Salt(128)
	require.NoError(t, err)

	kms, err := New(map[uint32][]byte{
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

			ks, err := kms.MasterKeys(ctx)
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
			kms.AddNewMasterKey(ctx, uint32(counter.Count()), mk)
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
			masterKeyID, dekID, ciphertext, err := kms.Encrypt(ctx, plaintext, []byte("laisky"))
			require.NoError(gt, err)

			t.Run("encrypt by id", func(t *testing.T) {
				gotcipher, err := kms.EncryptByID(ctx, plaintext, []byte("laisky"), masterKeyID, dekID)
				require.NoError(gt, err)
				require.NotEqual(gt, ciphertext, gotcipher)

				gotplain, err := kms.Decrypt(ctx, masterKeyID, dekID, gotcipher, []byte("laisky"))
				require.NoError(gt, err)
				require.Equal(gt, plaintext, gotplain)
			})

			t.Run("decrypt", func(t *testing.T) {
				gotplain, err := kms.Decrypt(ctx, masterKeyID, dekID, ciphertext, []byte("laisky"))
				require.NoError(gt, err)
				require.Equal(gt, plaintext, gotplain)
			})

			t.Run("decrypt with wrong add", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, masterKeyID, dekID, ciphertext, []byte("laisky123"))
				require.ErrorContains(gt, err, "message authentication failed")
			})

			t.Run("decrypt with wrong master key id", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, 0, dekID, ciphertext, []byte("laisky123"))
				require.ErrorContains(gt, err, "masterkey 0 not found")
			})

			t.Run("decrypt with wrong master key id", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, 2, dekID, ciphertext, []byte("laisky123"))
				require.ErrorContains(gt, err, "message authentication failed")
			})

			t.Run("decrypt with wrong dek key id", func(t *testing.T) {
				_, err = kms.Decrypt(ctx, masterKeyID, []byte("123"), ciphertext, []byte("laisky123"))
				require.ErrorContains(gt, err, "message authentication failed")
			})
		}()
	}

	wg.Wait()
}
