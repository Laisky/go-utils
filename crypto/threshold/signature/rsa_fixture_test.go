package signature

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/niclabs/tcrsa"
	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// thresholdFixture returns fresh, real 3-of-5 shares over a public TEST-ONLY
// modulus. These primes were generated with OpenSSL prime -generate -safe
// -bits 1025 and -bits 1023. They are never production defaults or secrets.
// Both prime factors and their Sophie Germain factors are revalidated. Sharing
// polynomials and signature proofs still use the real cryptographic RNG.
func thresholdFixture(t *testing.T) (tcrsa.KeyShareList, *tcrsa.KeyMeta) {
	t.Helper()
	const pHex = "01fd6a24797629e354fbcc5cfebd12ac13e32c60a4bf6f55b0a3c12e592f929305953f4ce9777f8f0d9f3c196894c65f89cca0952f93ad82b383096185b2e74c3a964a2753e6071627109ede0e781b7f3450a2a67637dc288bc1f878cb5311bed8abeb1bf2da276ee7e21399d896912f5553208de0aea8171692deaa61ba49140b"
	const qHex = "742b9065315744c7e798e4262d274c5b33dd6c76123b3d5501873a8a9fb14eb710324625bb4f53dea22318dbf01fc7410e90ef4335777ba525a2e395d1a25fe85730cae87c746444562ab01b7b8cfe0d271a9b19453f24e9a996ff4541074cb070e817392d682f6e2070adfe043695d09139fd3ce36cb63fd2e96643ad95b6e7"
	p, ok := new(big.Int).SetString(pHex, 16)
	require.True(t, ok)
	q, ok := new(big.Int).SetString(qHex, 16)
	require.True(t, ok)
	require.NotEqual(t, p, q)
	for _, prime := range []*big.Int{p, q} {
		require.True(t, prime.ProbablyPrime(64), "fixture must be prime")
		half := new(big.Int).Sub(prime, big.NewInt(1))
		half.Rsh(half, 1)
		require.True(t, half.ProbablyPrime(64), "fixture must be a safe prime")
	}
	require.GreaterOrEqual(t, new(big.Int).Mul(p, q).BitLen(), minRSAPublicKeyBits)
	shares, meta, err := tcrsa.NewKey(2049, 3, 5, &tcrsa.KeyMetaArgs{P: p, Q: q})
	require.NoError(t, err)
	require.GreaterOrEqual(t, meta.PublicKey.N.BitLen(), minRSAPublicKeyBits)
	return shares, meta
}

// TestThresholdFixtureMembership checks every threshold subset and verifies the
// joined signature independently with crypto/rsa. Below-threshold membership,
// tampered signatures and wrong messages must fail without weakening the key.
func TestThresholdFixtureMembership(t *testing.T) {
	t.Parallel()
	shares, meta := thresholdFixture(t)
	content := []byte("public threshold regression fixture")
	digest := sha256.Sum256(content)
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 4; j++ {
			for k := j + 1; k < 5; k++ {
				selected := tcrsa.KeyShareList{shares[i], shares[j], shares[k]}
				sig, err := SignBySHA256(bytes.NewReader(content), selected, meta)
				require.NoError(t, err)
				require.NoError(t, rsa.VerifyPKCS1v15(meta.PublicKey, crypto.SHA256, digest[:], sig))
				require.Error(t, VerifyBySHA256(strings.NewReader("different message"), meta.PublicKey, sig))
				sig[0] ^= 1
				require.Error(t, VerifyBySHA256(bytes.NewReader(content), meta.PublicKey, sig))
			}
		}
	}
	_, err := SignBySHA256(bytes.NewReader(content), shares[:2], meta)
	require.Error(t, err)
}

// TestNewKeySharesRandomIntegration retains production key-generation coverage
// as an explicit integration test. A child bounds the real random-prime search,
// is killed and reaped on deadline, and never substitutes randomness or primes.
// Run with GO_UTILS_RUN_THRESHOLD_KEYGEN=1 and go test -timeout=10m -run
// '^TestNewKeySharesRandomIntegration$' ./crypto/threshold/signature.
func TestNewKeySharesRandomIntegration(t *testing.T) {
	if os.Getenv("GO_UTILS_RUN_THRESHOLD_KEYGEN") != "1" {
		t.Skip("explicit random safe-prime integration; see package TESTING.md")
	}
	if os.Getenv("GO_UTILS_THRESHOLD_KEYGEN_CHILD") == "1" {
		shares, meta, err := NewKeyShares(5, 3, gcrypto.RSAPrikeyBits2048)
		require.NoError(t, err)
		require.Len(t, shares, 5)
		require.GreaterOrEqual(t, meta.PublicKey.N.BitLen(), minRSAPublicKeyBits)
		signature, err := SignBySHA256(strings.NewReader("production RNG integration"), shares[:3], meta)
		require.NoError(t, err)
		require.NoError(t, VerifyBySHA256(strings.NewReader("production RNG integration"), meta.PublicKey, signature))
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestNewKeySharesRandomIntegration$", "-test.timeout=8m")
	child.Env = append(os.Environ(), "GO_UTILS_THRESHOLD_KEYGEN_CHILD=1")
	output, err := child.CombinedOutput()
	require.NoError(t, ctx.Err(), "random key generation exceeded its bounded integration deadline")
	require.NoError(t, err, string(output))
}
