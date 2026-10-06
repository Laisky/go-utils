package crypto

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/stretchr/testify/require"
)

// TestSecurity42VerificationWarningBudget limits weak-hash warnings independently of the stored work factor.
func TestSecurity42VerificationWarningBudget(t *testing.T) {
	if serialized := os.Getenv("GO_UTILS_SECURITY42_HASH"); serialized != "" {
		start := time.Now()
		require.Error(t, VerifyHashedPassword([]byte("synthetic password"), serialized))
		require.GreaterOrEqual(t, time.Since(start), DefaultPasswordDelay)
		return
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	for _, algorithm := range []string{"sha1", "md5", "sha256"} {
		for _, iterations := range []int{3, 31} {
			t.Run(fmt.Sprintf("%s/%d", algorithm, iterations), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				child := exec.CommandContext(ctx, exe, "-test.run=^TestSecurity42VerificationWarningBudget$", "-test.timeout=8s")
				child.Env = append(os.Environ(), "GO_UTILS_SECURITY42_HASH="+fmt.Sprintf("%s.%d..00", algorithm, iterations))
				output, err := child.CombinedOutput()
				require.NoError(t, err, string(output))
				warnings := strings.Count(string(output), algorithm+" is not safe")
				expected := 1
				if algorithm == "sha256" {
					expected = 0
				}
				require.Equal(t, expected, warnings, "diagnostics must not scale with the work factor: %s", output)
			})
		}
	}
}

// security42HashFactory counts hash creation and can report a synthetic constructor error.
type security42HashFactory struct {
	calls int
	err   error
}

// String returns the test algorithm's diagnostic name.
func (h *security42HashFactory) String() string { return "security42-sha256" }

// Hasher returns a real SHA-256 implementation or the configured test error.
func (h *security42HashFactory) Hasher() (hash.Hash, error) {
	h.calls++
	if h.err != nil {
		return nil, h.err
	}
	return sha256.New(), nil
}

// TestSecurity42DigestCompatibility preserves digest results, input ownership, and validation failures.
func TestSecurity42DigestCompatibility(t *testing.T) {
	for _, n := range []int{1, 3, 31, MinPasswordHashIteration} {
		factory := new(security42HashFactory)
		password, salt := []byte("synthetic password"), []byte("salt")
		beforePassword, beforeSalt := string(password), string(salt)
		got, err := newHashedPasswordWithMinIteration(salt, password, factory, n, 1)
		require.NoError(t, err)
		expected := []byte(beforePassword + beforeSalt)
		for range n {
			sum := sha256.Sum256(expected)
			expected = sum[:]
		}
		require.Equal(t, expected, got.hashedPassword)
		require.Equal(t, n, factory.calls)
		require.Equal(t, beforePassword, string(password))
		require.Equal(t, beforeSalt, string(salt))
	}
	factory := &security42HashFactory{err: errors.New("synthetic factory failure")}
	_, err := newHashedPasswordWithMinIteration(nil, []byte("x"), factory, 3, 1)
	require.ErrorIs(t, err, factory.err)
	for _, n := range []int{0, -1, MaxPasswordHashIteration + 1} {
		factory := new(security42HashFactory)
		_, err := newHashedPasswordWithMinIteration(nil, []byte("x"), factory, n, 1)
		require.Error(t, err)
		require.Zero(t, factory.calls)
	}
}

// TestSecurity42LegacyVerification checks matching and changed legacy passwords
// against independent digest fixtures and retains the minimum verification delay.
func TestSecurity42LegacyVerification(t *testing.T) {
	for _, algorithm := range []gutils.HashType{gutils.HashTypeMD5, gutils.HashTypeSha1} {
		var h hash.Hash
		if algorithm == gutils.HashTypeMD5 {
			h = md5.New()
		} else {
			h = sha1.New()
		}
		value := []byte("synthetic password")
		for range 3 {
			h.Reset()
			_, err := h.Write(value)
			require.NoError(t, err)
			value = h.Sum(nil)
		}
		serialized := fmt.Sprintf("%s.3..%s", algorithm, hex.EncodeToString(value))
		start := time.Now()
		require.NoError(t, VerifyHashedPassword([]byte("synthetic password"), serialized))
		require.GreaterOrEqual(t, time.Since(start), DefaultPasswordDelay)
		start = time.Now()
		require.Error(t, VerifyHashedPassword([]byte("changed password"), serialized))
		require.GreaterOrEqual(t, time.Since(start), DefaultPasswordDelay)
	}
}

// TestSecurity42RejectsBeforeWarning exercises parser and size guards in fresh
// processes so earlier weak-hash uses cannot conceal unexpected diagnostics.
func TestSecurity42RejectsBeforeWarning(t *testing.T) {
	if serialized := os.Getenv("GO_UTILS_SECURITY42_INVALID"); serialized != "" {
		start := time.Now()
		require.Error(t, VerifyHashedPassword([]byte("synthetic password"), serialized))
		require.GreaterOrEqual(t, time.Since(start), DefaultPasswordDelay)
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	for name, serialized := range map[string]string{
		"zero": "sha1.0..00", "negative": "sha1.-1..00",
		"over-limit": fmt.Sprintf("sha1.%d..00", MaxPasswordHashIteration+1),
		"oversized":  "sha1.3.." + strings.Repeat("0", MaxHashedPasswordLength),
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, executable, "-test.run=^TestSecurity42RejectsBeforeWarning$", "-test.timeout=8s")
			child.Env = append(os.Environ(), "GO_UTILS_SECURITY42_INVALID="+serialized)
			output, err := child.CombinedOutput()
			require.NoError(t, ctx.Err())
			require.NoError(t, err, string(output))
			require.NotContains(t, string(output), "is not safe")
		})
	}
}
