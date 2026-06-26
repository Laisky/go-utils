package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConstantTimeStringEqual exercises the correctness contract of
// ConstantTimeStringEqual: it returns true iff sha256(candidate) equals the
// supplied digest, which (collisions aside) means candidate equals the secret
// that produced expectedHash. The cases below pin both return paths and a set
// of inputs that are easy to mishandle (empty strings, case folding, multibyte
// runes, embedded NUL bytes, and candidates longer/shorter than the secret).
func TestConstantTimeStringEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// secret is the configured value; expectedHash = sha256(secret).
		secret    string
		candidate string
		want      bool
	}{
		{
			name:      "exact ascii match",
			secret:    "correct horse battery staple",
			candidate: "correct horse battery staple",
			want:      true,
		},
		{
			name:      "mismatch with equal length",
			secret:    "password1234",
			candidate: "password1235",
			want:      false,
		},
		{
			name:      "single bit/char flip mismatch",
			secret:    "abcdef",
			candidate: "abcdeg",
			want:      false,
		},
		{
			name:      "case sensitive mismatch",
			secret:    "Secret",
			candidate: "secret",
			want:      false,
		},
		{
			name:      "empty candidate matches empty secret",
			secret:    "",
			candidate: "",
			want:      true,
		},
		{
			name:      "empty candidate against non-empty secret",
			secret:    "secret",
			candidate: "",
			want:      false,
		},
		{
			name:      "non-empty candidate against empty secret",
			secret:    "",
			candidate: "x",
			want:      false,
		},
		{
			name:      "candidate longer than secret",
			secret:    "abc",
			candidate: "abcd",
			want:      false,
		},
		{
			name:      "candidate far shorter than secret",
			secret:    strings.Repeat("a", 4096),
			candidate: "a",
			want:      false,
		},
		{
			name:      "unicode exact match",
			secret:    "пароль🔐",
			candidate: "пароль🔐",
			want:      true,
		},
		{
			name:      "unicode near miss",
			secret:    "пароль🔐",
			candidate: "пароль🔑",
			want:      false,
		},
		{
			name:      "trailing whitespace differs",
			secret:    "token",
			candidate: "token ",
			want:      false,
		},
		{
			// Guards against any C-style NUL truncation: the bytes after the
			// NUL must still participate in the comparison.
			name:      "embedded NUL byte exact match",
			secret:    "a\x00b",
			candidate: "a\x00b",
			want:      true,
		},
		{
			name:      "embedded NUL truncation is not a match",
			secret:    "a\x00b",
			candidate: "a",
			want:      false,
		},
		{
			name:      "candidate is secret with extra NUL suffix",
			secret:    "a",
			candidate: "a\x00",
			want:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expectedHash := sha256.Sum256([]byte(tt.secret))
			got := ConstantTimeStringEqual(tt.candidate, expectedHash)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestConstantTimeStringEqual_ZeroHashNeverMatches verifies that the all-zero
// digest (e.g. a zero-value [sha256.Size]byte from an unconfigured secret) is
// not matched by any input. No string hashes to all zeros, so an accidentally
// uninitialised expectedHash must reject every candidate rather than fail open.
func TestConstantTimeStringEqual_ZeroHashNeverMatches(t *testing.T) {
	t.Parallel()

	var zero [sha256.Size]byte
	for _, candidate := range []string{"", "admin", "\x00", strings.Repeat("\x00", sha256.Size)} {
		require.False(t, ConstantTimeStringEqual(candidate, zero),
			"zero-value digest must never accept candidate %q", candidate)
	}
}

// TestConstantTimeStringEqual_LengthIndependence documents the structural
// property the function exists for: because both operands of
// subtle.ConstantTimeCompare are always sha256.Size digests, the comparison
// never short-circuits on candidate length. The behavioural consequence is that
// candidates spanning a wide range of lengths still compare correctly and
// without panicking against a fixed secret's digest.
func TestConstantTimeStringEqual_LengthIndependence(t *testing.T) {
	t.Parallel()

	const secret = "the-configured-secret"
	expectedHash := sha256.Sum256([]byte(secret))

	for _, n := range []int{0, 1, len(secret) - 1, len(secret) + 1, 1024, 65536} {
		candidate := strings.Repeat("x", n)
		require.False(t, ConstantTimeStringEqual(candidate, expectedHash),
			"wrong candidate of length %d must not match", n)
	}

	// The correct secret still matches regardless of the surrounding length
	// probes above.
	require.True(t, ConstantTimeStringEqual(secret, expectedHash))
}

// TestConstantTimeStringEqual_Deterministic ensures the function is a pure
// comparison with no hidden state: repeated calls with identical arguments
// always yield the same result.
func TestConstantTimeStringEqual_Deterministic(t *testing.T) {
	t.Parallel()

	expectedHash := sha256.Sum256([]byte("repeatable"))
	for i := 0; i < 16; i++ {
		require.True(t, ConstantTimeStringEqual("repeatable", expectedHash))
		require.False(t, ConstantTimeStringEqual("repeatablE", expectedHash))
	}
}

// TestConstantTimeStringEqual_RandomSecretsRoundtrip checks the core invariant
// over random secrets: the secret matches its own digest while a flipped-byte
// neighbour does not.
func TestConstantTimeStringEqual_RandomSecretsRoundtrip(t *testing.T) {
	t.Parallel()

	for i := 0; i < 256; i++ {
		secret := make([]byte, 1+i%48)
		_, err := rand.Read(secret)
		require.NoError(t, err)

		expectedHash := sha256.Sum256(secret)
		require.True(t, ConstantTimeStringEqual(string(secret), expectedHash),
			"secret must match its own digest")

		// Flip one bit to build a guaranteed-different neighbour.
		neighbour := make([]byte, len(secret))
		copy(neighbour, secret)
		neighbour[len(neighbour)-1] ^= 0x01
		require.False(t, ConstantTimeStringEqual(string(neighbour), expectedHash),
			"single-bit-different candidate must not match")
	}
}

// FuzzConstantTimeStringEqual asserts the full equality contract across
// arbitrary input pairs: ConstantTimeStringEqual(candidate, sha256(secret))
// must equal candidate == secret. SHA-256 collisions are computationally
// infeasible, so any divergence the fuzzer surfaces is a real bug.
func FuzzConstantTimeStringEqual(f *testing.F) {
	seeds := []struct{ secret, candidate string }{
		{"", ""},
		{"secret", "secret"},
		{"secret", "Secret"},
		{"secret", ""},
		{"", "x"},
		{"a\x00b", "a"},
		{"пароль🔐", "пароль🔐"},
		{strings.Repeat("a", 4096), "a"},
	}
	for _, s := range seeds {
		f.Add(s.secret, s.candidate)
	}

	f.Fuzz(func(t *testing.T, secret, candidate string) {
		expectedHash := sha256.Sum256([]byte(secret))
		require.Equal(t, candidate == secret,
			ConstantTimeStringEqual(candidate, expectedHash))
	})
}

// BenchmarkConstantTimeStringEqual records the cost of a single comparison,
// which sits on the hot path for secret/token validation.
func BenchmarkConstantTimeStringEqual(b *testing.B) {
	expectedHash := sha256.Sum256([]byte("a-reasonably-long-shared-secret-value"))
	candidate := "a-reasonably-long-shared-secret-value"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ConstantTimeStringEqual(candidate, expectedHash)
	}
}
