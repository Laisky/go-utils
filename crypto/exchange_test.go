package crypto

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewDHKX verifies that two independent DHKX instances that exchange public keys derive the
// same shared key on both sides.
func TestNewDHKX(t *testing.T) {
	t.Parallel()

	alice, err := NewDHKX()
	require.NoError(t, err)

	bob, err := NewDHKX()
	require.NoError(t, err)

	alicePub, err := alice.PublicKey()
	require.NoError(t, err)
	bobPub, err := bob.PublicKey()
	require.NoError(t, err)

	aliceKey, err := alice.GenerateKey(bobPub)
	require.NoError(t, err)

	bobKey, err := bob.GenerateKey(alicePub)
	require.NoError(t, err)

	t.Logf("generate key: %+v", hex.EncodeToString(aliceKey))
	require.Equal(t, aliceKey, bobKey)
}

// ExampleDHKX demonstrates a DHKX key exchange in which Alice and Bob swap public keys and each
// derives the shared key with GenerateKey; it prints true because both keys are equal.
func ExampleDHKX() {
	alice, _ := NewDHKX()

	bob, _ := NewDHKX()

	alicePub, _ := alice.PublicKey()
	bobPub, _ := bob.PublicKey()

	aliceKey, _ := alice.GenerateKey(bobPub)
	bobKey, _ := bob.GenerateKey(alicePub)
	fmt.Println(reflect.DeepEqual(aliceKey, bobKey))
	// Output: true
}

// TestNewEcdh verifies that on the P-256, P-384 and P-521 curves two ECDH instances derive the same
// shared key from each other's public keys, and that NewEcdh rejects an unknown curve name with an
// "unsupport curve" error.
func TestNewEcdh(t *testing.T) {
	t.Parallel()

	for _, curve := range []ECDSACurve{
		ECDSACurveP256,
		ECDSACurveP384,
		ECDSACurveP521,
	} {
		t.Run(string(curve), func(t *testing.T) {
			alice, err := NewEcdh(curve)
			require.NoError(t, err)

			bob, err := NewEcdh(curve)
			require.NoError(t, err)

			_, err = NewEcdh(ECDSACurve("yahoo"))
			require.ErrorContains(t, err, "unsupport curve yahoo")

			alicePub, err := alice.PublicKey()
			require.NoError(t, err)
			bobPub, err := bob.PublicKey()
			require.NoError(t, err)

			aliceKey, err := alice.GenerateKey(bobPub)
			require.NoError(t, err)

			bobKey, err := bob.GenerateKey(alicePub)
			require.NoError(t, err)

			require.Equal(t, aliceKey, bobKey)
		})
	}
}

// TestECDH_GenerateKey_EmptyInput verifies that ECDH.GenerateKey returns a "peer public key is
// empty" error instead of panicking when the peer public key is nil or an empty slice.
func TestECDH_GenerateKey_EmptyInput(t *testing.T) {
	t.Parallel()

	alice, err := NewEcdh(ECDSACurveP256)
	require.NoError(t, err)

	// empty peer public key must return error, not panic
	_, err = alice.GenerateKey(nil)
	require.ErrorContains(t, err, "peer public key is empty")

	_, err = alice.GenerateKey([]byte{})
	require.ErrorContains(t, err, "peer public key is empty")
}

// ExampleNewEcdh demonstrates a P-256 ECDH key exchange in which Alice and Bob swap public keys and
// each derives the shared key with GenerateKey; it prints true because both keys are equal.
func ExampleNewEcdh() {
	alice, _ := NewEcdh(ECDSACurveP256)

	bob, _ := NewEcdh(ECDSACurveP256)

	alicePub, _ := alice.PublicKey()
	bobPub, _ := bob.PublicKey()

	aliceKey, _ := alice.GenerateKey(bobPub)
	bobKey, _ := bob.GenerateKey(alicePub)
	fmt.Println(reflect.DeepEqual(aliceKey, bobKey))
	// Output: true
}

// Benchmark_aggrements measures one complete key agreement, in which both peers call GenerateKey on
// the other's public key, for the deprecated DHKX (default MODP group 14) and for P-256 ECDH. The
// pasted result below is from a sample run:
//
// cpu: AMD Ryzen 7 5700G with Radeon Graphics
// Benchmark_aggrements/dhkx-16         	     147	   8003088 ns/op	   25852 B/op	      62 allocs/op
// Benchmark_aggrements/ecdh-16         	   12034	     99670 ns/op	     752 B/op	      12 allocs/op
// PASS
func Benchmark_aggrements(b *testing.B) {
	b.Run("dhkx", func(b *testing.B) {
		alice, err := NewDHKX()
		require.NoError(b, err)

		bob, err := NewDHKX()
		require.NoError(b, err)

		alicePub, err := alice.PublicKey()
		require.NoError(b, err)
		bobPub, err := bob.PublicKey()
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ka, err := alice.GenerateKey(bobPub)
			require.NoError(b, err)

			kb, err := bob.GenerateKey(alicePub)
			require.NoError(b, err)

			require.Equal(b, ka, kb)
		}
	})

	b.Run("ecdh", func(b *testing.B) {
		alice, err := NewEcdh(ECDSACurveP256)
		require.NoError(b, err)

		bob, err := NewEcdh(ECDSACurveP256)
		require.NoError(b, err)

		alicePub, err := alice.PublicKey()
		require.NoError(b, err)
		bobPub, err := bob.PublicKey()
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ka, err := alice.GenerateKey(bobPub)
			require.NoError(b, err)

			kb, err := bob.GenerateKey(alicePub)
			require.NoError(b, err)

			require.Equal(b, ka, kb)
		}
	})
}
