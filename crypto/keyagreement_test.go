package crypto

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDHKX(t *testing.T) {
	t.Parallel()

	alice, err := NewDHKX()
	require.NoError(t, err)

	bob, err := NewDHKX()
	require.NoError(t, err)

	alicePub := alice.PublicKey()
	bobPub := bob.PublicKey()

	aliceKey, err := alice.GenerateKey(bobPub)
	require.NoError(t, err)

	bobKey, err := bob.GenerateKey(alicePub)
	require.NoError(t, err)

	t.Logf("generate key: %+v", hex.EncodeToString(aliceKey))
	require.Equal(t, aliceKey, bobKey)
}

func ExampleDHKX() {
	alice, _ := NewDHKX()

	bob, _ := NewDHKX()

	alicePub := alice.PublicKey()
	bobPub := bob.PublicKey()

	aliceKey, _ := alice.GenerateKey(bobPub)
	bobKey, _ := bob.GenerateKey(alicePub)
	fmt.Println(reflect.DeepEqual(aliceKey, bobKey))
	// Output: true
}

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

			alicePub := alice.PublicKey()
			bobPub := bob.PublicKey()

			aliceKey, err := alice.GenerateKey(bobPub)
			require.NoError(t, err)

			bobKey, err := bob.GenerateKey(alicePub)
			require.NoError(t, err)

			require.Equal(t, aliceKey, bobKey)
		})
	}
}

func ExampleNewEcdh() {
	alice, _ := NewEcdh(ECDSACurveP256)

	bob, _ := NewEcdh(ECDSACurveP256)

	alicePub := alice.PublicKey()
	bobPub := bob.PublicKey()

	aliceKey, _ := alice.GenerateKey(bobPub)
	bobKey, _ := bob.GenerateKey(alicePub)
	fmt.Println(reflect.DeepEqual(aliceKey, bobKey))
	// Output: true
}

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

		alicePub := alice.PublicKey()
		bobPub := bob.PublicKey()

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

		alicePub := alice.PublicKey()
		bobPub := bob.PublicKey()

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
