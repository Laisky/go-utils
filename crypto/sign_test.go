package crypto

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"strings"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3/group/edwards25519"
	dediskey "go.dedis.ch/kyber/v3/util/key"

	"github.com/Laisky/go-utils/v6/log"
)

// TestECDSAKeySerializer verifies that ECDSA keys survive the package's key serializers on every
// supported curve: the PEM (Prikey2Pem/Pem2Prikey, Pubkey2Pem/Pem2Pubkey) and DER (Prikey2Der/
// Der2Prikey, Pubkey2Der/Der2Pubkey) round-trips return keys equal to the originals, a signature
// made by the original private key verifies with the decoded public key and vice versa, the ES256
// signature encodings round-trip, and truncated or non-PEM serialized keys are rejected.
func TestECDSAKeySerializer(t *testing.T) {
	t.Parallel()

	for _, curve := range []ECDSACurve{ECDSACurveP256, ECDSACurveP384, ECDSACurveP521} {
		t.Run(string(curve), func(t *testing.T) {
			t.Parallel()

			priKey, err := NewECDSAPrikey(curve)
			require.NoError(t, err)

			priPem, err := Prikey2Pem(priKey)
			require.NoError(t, err)
			require.True(t, bytes.HasPrefix(priPem, []byte("-----BEGIN PRIVATE KEY-----")))
			pubPem, err := Pubkey2Pem(&priKey.PublicKey)
			require.NoError(t, err)
			require.True(t, bytes.HasPrefix(pubPem, []byte("-----BEGIN PUBLIC KEY-----")))
			priDer, err := Prikey2Der(priKey)
			require.NoError(t, err)
			pubDer, err := Pubkey2Der(&priKey.PublicKey)
			require.NoError(t, err)

			decodedPri := requireECDSAPrikey(t, priPem, Pem2Prikey)
			require.True(t, priKey.Equal(decodedPri), "PEM private key round-trip must be lossless")
			require.True(t, priKey.Equal(requireECDSAPrikey(t, priDer, Der2Prikey)),
				"DER private key round-trip must be lossless")

			decodedPub := requireECDSAPubkey(t, pubPem, Pem2Pubkey)
			require.True(t, priKey.PublicKey.Equal(decodedPub), "PEM public key round-trip must be lossless")
			require.True(t, priKey.PublicKey.Equal(requireECDSAPubkey(t, pubDer, Der2Pubkey)),
				"DER public key round-trip must be lossless")
			require.True(t, priKey.PublicKey.Equal(Prikey2Pubkey(decodedPri)))

			content := []byte("hello, world")
			r, s, err := SignByECDSAWithSHA256(priKey, content)
			require.NoError(t, err)
			require.True(t, VerifyByECDSAWithSHA256(decodedPub, content, r, s))
			require.False(t, VerifyByECDSAWithSHA256(decodedPub, []byte("hello, world!"), r, s))

			r2, s2, err := SignByECDSAWithSHA256(decodedPri, content)
			require.NoError(t, err)
			require.True(t, VerifyByECDSAWithSHA256(&priKey.PublicKey, content, r2, s2))

			gotR, gotS, err := DecodeES256SignByBase64(EncodeES256SignByBase64(r, s))
			require.NoError(t, err)
			require.Zero(t, r.Cmp(gotR))
			require.Zero(t, s.Cmp(gotS))
			gotR, gotS, err = DecodeES256SignByHex(EncodeES256SignByHex(r, s))
			require.NoError(t, err)
			require.Zero(t, r.Cmp(gotR))
			require.Zero(t, s.Cmp(gotS))

			_, err = Pem2Prikey([]byte("not a pem encoded key"))
			require.Error(t, err)
			_, err = Pem2Pubkey([]byte("not a pem encoded key"))
			require.Error(t, err)
			_, err = Der2Prikey(priDer[:len(priDer)/2])
			require.Error(t, err)
			_, err = Der2Pubkey(pubDer[:len(pubDer)/2])
			require.Error(t, err)
		})
	}
}

// requireECDSAPrikey decodes encoded with decode and fails the test unless the result is an
// *ecdsa.PrivateKey. It returns the decoded private key.
func requireECDSAPrikey(t *testing.T, encoded []byte,
	decode func([]byte) (crypto.PrivateKey, error)) *ecdsa.PrivateKey {
	t.Helper()

	decoded, err := decode(encoded)
	require.NoError(t, err)
	key, ok := decoded.(*ecdsa.PrivateKey)
	require.Truef(t, ok, "decoded private key has type %T, want *ecdsa.PrivateKey", decoded)
	return key
}

// requireECDSAPubkey decodes encoded with decode and fails the test unless the result is an
// *ecdsa.PublicKey. It returns the decoded public key.
func requireECDSAPubkey(t *testing.T, encoded []byte,
	decode func([]byte) (crypto.PublicKey, error)) *ecdsa.PublicKey {
	t.Helper()

	decoded, err := decode(encoded)
	require.NoError(t, err)
	key, ok := decoded.(*ecdsa.PublicKey)
	require.Truef(t, ok, "decoded public key has type %T, want *ecdsa.PublicKey", decoded)
	return key
}

// TestECDSAVerify verifies SignByECDSAWithSHA256 and VerifyByECDSAWithSHA256 with P-256 keys on
// random messages of 1, 1024 and 10240 bytes: verification succeeds with the signing key and fails
// for a modified message or for a signature made by a different key.
func TestECDSAVerify(t *testing.T) {
	t.Parallel()

	priKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	priKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	for _, plainLen := range []int{
		1, 1024, 10240,
	} {
		plain, err := Salt(plainLen)
		require.NoError(t, err)
		t.Run(fmt.Sprintf("plainLen=%d", plainLen), func(t *testing.T) {
			t.Parallel()

			t.Run("correct key", func(t *testing.T) {
				r, s, err := SignByECDSAWithSHA256(priKey, plain)
				require.NoError(t, err)
				require.True(t, VerifyByECDSAWithSHA256(&priKey.PublicKey, plain, r, s))
			})

			t.Run("incorrect plain", func(t *testing.T) {
				r, s, err := SignByECDSAWithSHA256(priKey, plain)
				require.NoError(t, err)
				require.False(t, VerifyByECDSAWithSHA256(&priKey.PublicKey, append(plain, '2'), r, s))
			})

			t.Run("incorrect key", func(t *testing.T) {
				r, s, err := SignByECDSAWithSHA256(priKey2, plain)
				require.NoError(t, err)
				require.False(t, VerifyByECDSAWithSHA256(&priKey.PublicKey, plain, r, s))
			})
		})
	}
}

// TestRSAVerify verifies SignByRSAPKCS1v15WithSHA256 and VerifyByRSAPKCS1v15WithSHA256 with
// RSA-2048 keys on random messages of 1, 1024 and 10240 bytes: verification succeeds with the
// signing key and returns a "verification error" for a modified message or for a signature made by
// a different key.
func TestRSAVerify(t *testing.T) {
	t.Parallel()

	priKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	priKey2, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	for _, plainLen := range []int{
		1, 1024, 10240,
	} {
		plain, err := Salt(plainLen)
		require.NoError(t, err)
		t.Run(fmt.Sprintf("plainLen=%d", plainLen), func(t *testing.T) {
			t.Parallel()

			t.Run("correct key", func(t *testing.T) {
				sig, err := SignByRSAPKCS1v15WithSHA256(priKey, plain)
				require.NoError(t, err)

				err = VerifyByRSAPKCS1v15WithSHA256(&priKey.PublicKey, plain, sig)
				require.NoError(t, err)
			})

			t.Run("incorrect plain", func(t *testing.T) {
				sig, err := SignByRSAPKCS1v15WithSHA256(priKey, plain)
				require.NoError(t, err)

				err = VerifyByRSAPKCS1v15WithSHA256(&priKey.PublicKey, append(plain, '2'), sig)
				require.ErrorContains(t, err, "verification error")
			})

			t.Run("incorrect key", func(t *testing.T) {
				sig, err := SignByRSAPKCS1v15WithSHA256(priKey2, plain)
				require.NoError(t, err)

				err = VerifyByRSAPKCS1v15WithSHA256(&priKey.PublicKey, plain, sig)
				require.ErrorContains(t, err, "verification error")
			})
		})
	}
}

// TestRSAPSSVerify verifies SignByRSAPSSWithSHA256 and VerifyByRSAPSSWithSHA256 with RSA-2048 keys
// on random messages of 1, 1024 and 10240 bytes: verification succeeds with the signing key,
// returns a "verification error" for a modified message or a different key, and two signatures of
// the same message differ because PSS signing is randomized.
func TestRSAPSSVerify(t *testing.T) {
	t.Parallel()

	priKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	priKey2, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	for _, plainLen := range []int{
		1, 1024, 10240,
	} {
		plain, err := Salt(plainLen)
		require.NoError(t, err)
		t.Run(fmt.Sprintf("plainLen=%d", plainLen), func(t *testing.T) {
			t.Parallel()

			t.Run("correct key", func(t *testing.T) {
				sig, err := SignByRSAPSSWithSHA256(priKey, plain)
				require.NoError(t, err)

				err = VerifyByRSAPSSWithSHA256(&priKey.PublicKey, plain, sig)
				require.NoError(t, err)
			})

			t.Run("incorrect plain", func(t *testing.T) {
				sig, err := SignByRSAPSSWithSHA256(priKey, plain)
				require.NoError(t, err)

				err = VerifyByRSAPSSWithSHA256(&priKey.PublicKey, append(plain, '2'), sig)
				require.ErrorContains(t, err, "verification error")
			})

			t.Run("incorrect key", func(t *testing.T) {
				sig, err := SignByRSAPSSWithSHA256(priKey2, plain)
				require.NoError(t, err)
				err = VerifyByRSAPSSWithSHA256(&priKey.PublicKey, plain, sig)
				require.ErrorContains(t, err, "verification error")
			})

			t.Run("indetermistic sig", func(t *testing.T) {
				sig1, err := SignByRSAPSSWithSHA256(priKey, plain)
				require.NoError(t, err)
				sig2, err := SignByRSAPSSWithSHA256(priKey, plain)
				require.NoError(t, err)

				require.NotEqual(t, sig1, sig2)
			})
		})
	}
}

// ExampleSignByECDSAWithSHA256 demonstrates signing content with SignByECDSAWithSHA256, verifying
// it with VerifyByECDSAWithSHA256, and encoding the signature with EncodeES256SignByBase64 and
// decoding it back, and shows that verification fails for altered content or for a signature made
// with another key.
func ExampleSignByECDSAWithSHA256() {
	priKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Shared.Panic("generate key", zap.Error(err))
	}
	priKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Shared.Panic("generate key", zap.Error(err))
	}

	// case: correct key
	cnt := []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err := SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		log.Shared.Panic("sign", zap.Error(err))
	}
	if !VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		log.Shared.Panic("verify failed")
	}

	// generate string
	encoded := EncodeES256SignByBase64(r, s)
	if _, _, err = DecodeES256SignByBase64(encoded); err != nil {
		log.Shared.Panic("encode and decode", zap.Error(err))
	}

	// case: incorrect cnt
	cnt = []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err = SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		log.Shared.Panic("sign", zap.Error(err))
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, append(cnt, '2'), r, s) {
		log.Shared.Panic("should not verify")
	}

	// case: incorrect key
	r, s, err = SignByECDSAWithSHA256(priKey2, cnt)
	if err != nil {
		log.Shared.Panic("sign", zap.Error(err))
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		log.Shared.Panic("should not verify")
	}
}

// func Test_expandAesSecret(t *testing.T) {
// 	type args struct {
// 		secret []byte
// 	}
// 	tests := []struct {
// 		name string
// 		args args
// 		want int
// 	}{
// 		{"0", args{[]byte("")}, 16},
// 		{"1", args{[]byte("1")}, 16},
// 		{"2", args{[]byte("12")}, 16},
// 		{"3", args{[]byte("14124")}, 16},
// 		{"4", args{[]byte("1535435535")}, 16},
// 		{"5", args{[]byte("   43242341")}, 16},
// 		{"6", args{[]byte("1111111111111111")}, 16},
// 		{"7", args{[]byte("11111111111111111")}, 24},
// 		{"8", args{[]byte("11111111111111111   ")}, 24},
// 		{"9", args{[]byte("11111111111111111   23423 4324   ")}, 32},
// 		{"10", args{[]byte("11111111111111111   23423 4324   111")}, 32},
// 		{"11", args{[]byte("11111111111111111   23423 4324   111414124")}, 32},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if got := expandAesSecret(tt.args.secret); len(got) != tt.want {
// 				t.Errorf("expandAesSecret() = (%d)%v, want %v", len(got), got, tt.want)
// 			}
// 		})
// 	}

// 	// race
// 	var pool errgroup.Group
// 	secret := make([]byte, 5, 10)
// 	for i := 0; i < 17; i++ {
// 		pool.Go(func() error {
// 			expandAesSecret(secret)
// 			return nil
// 		})
// 	}

// 	if err := pool.Wait(); err != nil {
// 		t.Fatalf("%+v", err)
// 	}
// }

// TestSignReaderByEd25519WithSHA256 verifies that SignReaderByEd25519WithSHA256 and
// VerifyReaderByEd25519WithSHA256 round-trip over a 100 MiB random stream, and that verification
// fails for a different public key or a malformed signature.
func TestSignReaderByEd25519WithSHA256(t *testing.T) {
	t.Parallel()

	raw, err := Salt(100 * 1024 * 1024)
	require.NoError(t, err)

	prikey, err := NewEd25519Prikey()
	require.NoError(t, err)

	sig, err := SignReaderByEd25519WithSHA256(prikey, bytes.NewReader(raw))
	require.NoError(t, err)

	pubkey := Prikey2Pubkey(prikey).(ed25519.PublicKey)
	err = VerifyReaderByEd25519WithSHA256(pubkey, bytes.NewReader(raw), sig)
	require.NoError(t, err)

	t.Run("false pubkey", func(t *testing.T) {
		prikey, err := NewEd25519Prikey()
		require.NoError(t, err)
		pubkey := Prikey2Pubkey(prikey).(ed25519.PublicKey)

		err = VerifyReaderByEd25519WithSHA256(pubkey, bytes.NewReader(raw), sig)
		require.ErrorContains(t, err, "invalid signature")
	})

	t.Run("false sig", func(t *testing.T) {
		falseSig := []byte("2l3fj238f83fu")
		err := VerifyReaderByEd25519WithSHA256(pubkey, bytes.NewReader(raw), falseSig)
		require.Error(t, err)

		errmsg := err.Error()
		// go1.23 raise "invalid signature", go1.24 raise "bad signature"
		if !strings.Contains(errmsg, "bad signature") &&
			!strings.Contains(errmsg, "invalid signature") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestVerifyBySchnorrSha256 verifies Schnorr signing over the edwards25519 suite: public and
// private keys survive binary marshal and unmarshal, a signature from the unmarshaled private key
// verifies with VerifyBySchnorrSha256, and verification returns "invalid signature" whenever the
// public key does not match the signing key.
func TestVerifyBySchnorrSha256(t *testing.T) {
	t.Parallel()

	suite := edwards25519.NewBlakeSHA256Ed25519()
	keyPair := dediskey.NewKeyPair(suite)

	content := []byte("hello, world")

	t.Run("pubkey marshal & unmarshal", func(t *testing.T) {
		pub, err := keyPair.Public.MarshalBinary()
		require.NoError(t, err)

		pub2 := suite.Point()
		err = pub2.UnmarshalBinary(pub)
		require.NoError(t, err)

		require.True(t, keyPair.Public.Equal(pub2))
	})

	t.Run("prikey marshal & unmarshal", func(t *testing.T) {
		priBytes, err := keyPair.Private.MarshalBinary()
		require.NoError(t, err)

		pri2 := suite.Scalar()
		pri2.UnmarshalBinary(priBytes)
		require.NoError(t, err)

		t.Run("sign & verify", func(t *testing.T) {
			sig, err := SignBySchnorrSha256(suite, pri2, bytes.NewReader(content))
			require.NoError(t, err)

			err = VerifyBySchnorrSha256(suite, keyPair.Public, bytes.NewReader(content), sig)
			require.NoError(t, err)
		})

		t.Run("sign & invalid verify", func(t *testing.T) {
			keyPair2 := dediskey.NewKeyPair(suite)

			sig, err := SignBySchnorrSha256(suite, keyPair.Private, bytes.NewReader(content))
			require.NoError(t, err)

			err = VerifyBySchnorrSha256(suite, keyPair2.Public, bytes.NewReader(content), sig)
			require.ErrorContains(t, err, "invalid signature")
		})

		t.Run("sign & invalid verify", func(t *testing.T) {
			keyPair2 := dediskey.NewKeyPair(suite)

			sig, err := SignBySchnorrSha256(suite, keyPair2.Private, bytes.NewReader(content))
			require.NoError(t, err)

			err = VerifyBySchnorrSha256(suite, keyPair.Public, bytes.NewReader(content), sig)
			require.ErrorContains(t, err, "invalid signature")
		})
	})
}

// Benchmark_Sign measures signing a 4 KiB random message with RSA-2048 and RSA-4096 (PKCS#1 v1.5
// with SHA-256), ECDSA P-256 and P-384 (SHA-256), pure Ed25519, and Schnorr over edwards25519
// (SHA-256), excluding key generation from the timings.
//
// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils/v6/crypto
// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// Benchmark_Sign
// Benchmark_Sign/sign_rsa-2048_4k
// Benchmark_Sign/sign_rsa-2048_4k-104         	      98	  12063393 ns/op	     896 B/op	       5 allocs/op
// Benchmark_Sign/sign_rsa-4096_4k
// Benchmark_Sign/sign_rsa-4096_4k-104         	      22	  53844454 ns/op	   38656 B/op	      56 allocs/op
// Benchmark_Sign/sign_ecdsa-P256_4k
// Benchmark_Sign/sign_ecdsa-P256_4k-104       	   10752	    114306 ns/op	    2719 B/op	      37 allocs/op
// Benchmark_Sign/sign_ecdsa-P384_4k
// Benchmark_Sign/sign_ecdsa-P384_4k-104       	     253	   4663023 ns/op	    2920 B/op	      38 allocs/op
// Benchmark_Sign/sign_ed25519_4k
// Benchmark_Sign/sign_ed25519_4k-104				2299	    521442 ns/op	      64 B/op	       1 allocs/op
// Benchmark_Sign/sign_schnorr-ed25519_4k
// Benchmark_Sign/sign_schnorr-ed25519_4k-104  	     493	   2252567 ns/op	    3822 B/op	      48 allocs/op
// PASS
// coverage: 1.7% of statements
// ok  	github.com/Laisky/go-utils/v6/crypto	16.896s
func Benchmark_Sign(b *testing.B) {
	raw4k, err := Salt(4 * 1024)
	require.NoError(b, err)

	b.Run("sign rsa-2048 4k", func(b *testing.B) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits2048)
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := SignByRSAWithSHA256(prikey, raw4k)
			require.NoError(b, err)
		}
	})

	b.Run("sign rsa-4096 4k", func(b *testing.B) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := SignByRSAWithSHA256(prikey, raw4k)
			require.NoError(b, err)
		}
	})

	b.Run("sign ecdsa-P256 4k", func(b *testing.B) {
		prikey, err := NewECDSAPrikey(ECDSACurveP256)
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := SignByECDSAWithSHA256(prikey, raw4k)
			require.NoError(b, err)
		}
	})

	b.Run("sign ecdsa-P384 4k", func(b *testing.B) {
		prikey, err := NewECDSAPrikey(ECDSACurveP384)
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := SignByECDSAWithSHA256(prikey, raw4k)
			require.NoError(b, err)
		}
	})

	b.Run("sign ed25519 4k", func(b *testing.B) {
		prikey, err := NewEd25519Prikey()
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := prikey.Sign(rand.Reader, raw4k, crypto.Hash(0))
			require.NoError(b, err)
		}
	})

	b.Run("sign schnorr-ed25519 4k", func(b *testing.B) {
		suite := edwards25519.NewBlakeSHA256Ed25519()
		keyPair := dediskey.NewKeyPair(suite)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := SignBySchnorrSha256(suite, keyPair.Private, bytes.NewReader(raw4k))
			require.NoError(b, err)
		}
	})

}

// TestVerifyByEd25519 verifies that a legacy Ed25519-over-SHA512 signature from
// SignByEd25519WithSHA512 verifies with VerifyByEd25519WithSHA512, and that a malformed signature
// is rejected with "invalid signature" under both the signing key and an unrelated key.
func TestVerifyByEd25519(t *testing.T) {
	t.Parallel()

	prikey, err := NewEd25519Prikey()
	require.NoError(t, err)
	pubkey := prikey.Public().(ed25519.PublicKey)

	content := []byte("hello, world")

	sig, err := SignByEd25519WithSHA512(prikey, bytes.NewReader(content))
	require.NoError(t, err)

	err = VerifyByEd25519WithSHA512(pubkey, bytes.NewReader(content), sig)
	require.NoError(t, err)

	t.Run("invalid sig", func(t *testing.T) {
		err := VerifyByEd25519WithSHA512(pubkey, bytes.NewReader(content), []byte("2l3fj238f83"))
		require.ErrorContains(t, err, "invalid signature")
	})

	t.Run("invalid key", func(t *testing.T) {
		prikey, err := NewEd25519Prikey()
		require.NoError(t, err)
		pubkey := prikey.Public().(ed25519.PublicKey)

		err = VerifyByEd25519WithSHA512(pubkey, bytes.NewReader(content), []byte("2l3fj238f83"))
		require.ErrorContains(t, err, "invalid signature")
	})
}
