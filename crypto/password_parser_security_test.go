package crypto

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	gutils "github.com/Laisky/go-utils/v6"
)

// hostilePasswordRecord is a record that must be rejected before any key derivation.
type hostilePasswordRecord struct {
	name   string
	record string
	// errContains optionally pins the VerifyHashedPassword error.
	errContains string
}

// hostilePasswordRecords returns malformed, oversized, out-of-bound and
// algorithm-confused records. Every key field carries a CANARY marker so tests
// can check that errors never echo the stored record.
func hostilePasswordRecords() []hostilePasswordRecord {
	salt := argon2idBase64.EncodeToString([]byte("0123456789abcdef"))
	key := argon2idBase64.EncodeToString([]byte("CANARY-CANARY-CANARY-CANARY-CANA"))
	record := func(version, params, salt, key string) string {
		return "$argon2id$" + version + "$" + params + "$" + salt + "$" + key
	}
	withParams := func(params string) string { return record("v=19", params, salt, key) }
	withSalt := func(s string) string { return record("v=19", "m=65536,t=3,p=4", s, key) }
	withKey := func(k string) string { return record("v=19", "m=65536,t=3,p=4", salt, k) }
	valid := withParams("m=65536,t=3,p=4")

	// A canonical 16-byte encoding ends in a character whose low four bits are
	// zero; the next alphabet character sets a trailing bit.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	lastIdx := strings.IndexByte(alphabet, salt[len(salt)-1])
	trailingBits := salt[:len(salt)-1] + string(alphabet[lastIdx+1])
	urlSafe := strings.NewReplacer("/", "_", "+", "-").Replace(argon2idBase64.EncodeToString(bytes.Repeat([]byte{0xff}, 16)))

	return []hostilePasswordRecord{
		{name: "memory max uint32", record: withParams("m=4294967295,t=3,p=4")},
		{name: "memory above cap", record: withParams("m=262145,t=3,p=4")},
		{name: "memory overflows uint32", record: withParams("m=4294967296,t=3,p=4")},
		{name: "memory 11 digits", record: withParams("m=99999999999,t=3,p=4")},
		{name: "memory below 8p", record: withParams("m=31,t=3,p=4")},
		{name: "memory zero", record: withParams("m=0,t=3,p=4")},
		{name: "time above cap", record: withParams("m=65536,t=11,p=4")},
		{name: "time max uint32", record: withParams("m=65536,t=4294967295,p=4")},
		{name: "time zero", record: withParams("m=65536,t=0,p=4")},
		{name: "parallelism above cap", record: withParams("m=65536,t=3,p=17")},
		{name: "parallelism wraps uint8 to 4", record: withParams("m=65536,t=3,p=260")},
		{name: "parallelism zero", record: withParams("m=65536,t=3,p=0")},
		{name: "leading zero", record: withParams("m=065536,t=3,p=4")},
		{name: "plus sign", record: withParams("m=+65536,t=3,p=4")},
		{name: "whitespace", record: withParams("m=65536, t=3,p=4")},
		{name: "reordered", record: withParams("t=3,m=65536,p=4")},
		{name: "missing parallelism", record: withParams("m=65536,t=3")},
		{name: "extra keyid", record: withParams("m=65536,t=3,p=4,keyid=Q0FOQVJZ")},
		{name: "empty params", record: withParams("")},
		{name: "version 16", record: record("v=16", "m=65536,t=3,p=4", salt, key)},
		{name: "version leading zero", record: record("v=019", "m=65536,t=3,p=4", salt, key)},
		{name: "version uppercase", record: record("V=19", "m=65536,t=3,p=4", salt, key)},
		{name: "version missing", record: "$argon2id$m=65536,t=3,p=4$" + salt + "$" + key},
		{name: "padded salt", record: withSalt(salt + "==")},
		{name: "padded hash", record: withKey(key + "=")},
		{name: "salt trailing bits", record: withSalt(trailingBits)},
		{name: "salt line break", record: withSalt(salt[:10] + "\n" + salt[10:])},
		{name: "salt url alphabet", record: withSalt(urlSafe)},
		{name: "empty salt", record: withSalt("")},
		{name: "empty hash", record: withKey("")},
		{name: "salt 15 bytes", record: withSalt(argon2idBase64.EncodeToString(make([]byte, 15)))},
		{name: "salt 65 bytes", record: withSalt(argon2idBase64.EncodeToString(make([]byte, 65)))},
		{name: "hash 15 bytes", record: withKey(argon2idBase64.EncodeToString(make([]byte, 15)))},
		{name: "hash 65 bytes", record: withKey(argon2idBase64.EncodeToString(make([]byte, 65)))},
		{name: "extra field", record: valid + "$Q0FOQVJZ"},
		{name: "trailing dollar", record: valid + "$"},
		{name: "oversized", record: valid + strings.Repeat("A", MaxHashedPasswordLength),
			errContains: "hashedPassword is too long"},
		{name: "argon2i", record: strings.Replace(valid, "$argon2id$", "$argon2i$", 1),
			errContains: "unsupported password hash format"},
		{name: "argon2d", record: strings.Replace(valid, "$argon2id$", "$argon2d$", 1),
			errContains: "unsupported password hash format"},
		{name: "uppercase argon2id", record: strings.Replace(valid, "$argon2id$", "$ARGON2ID$", 1),
			errContains: "unsupported password hash format"},
		{name: "argon2id without leading dollar", record: valid[1:],
			errContains: "unsupported password hash format"},
		{name: "scrypt", record: "$scrypt$ln=16,r=8,p=1$" + salt + "$" + key,
			errContains: "unsupported password hash format"},
		{name: "bcrypt", record: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
			errContains: "unsupported password hash format"},
		{name: "legacy with dollar", record: "sha256$.10000.00.00",
			errContains: "unsupported password hash format"},
		{name: "legacy named argon2id", record: "argon2id.3.00.00"},
	}
}

// TestPasswordSecurity_HostileRecordsRejectedBeforeKDF verifies that malformed,
// oversized, hostile-cost and algorithm-confused records are rejected by the
// strict parser, by PasswordHashNeedsRehash and by VerifyHashedPassword without
// running Argon2id and without echoing the record; regression for issue #93.
//
// It is deliberately not parallel at the top level so that no other test can
// derive keys while the derivation counter is observed.
func TestPasswordSecurity_HostileRecordsRejectedBeforeKDF(t *testing.T) {
	records := hostilePasswordRecords()
	before := argon2idDerivations.Load()

	for _, tc := range records {
		if strings.HasPrefix(tc.record, argon2idPHCPrefix) {
			_, err := parseArgon2idRecord(tc.record, argon2idMinSaltLength)
			require.Error(t, err, tc.name)
			require.NotContains(t, err.Error(), "Q0FOQVJZ", tc.name)
		}
		_, err := PasswordHashNeedsRehash(tc.record)
		require.Error(t, err, tc.name)
		require.NotContains(t, err.Error(), "Q0FOQVJZ", tc.name)
	}

	// The verifications are sleep-bound by DefaultPasswordDelay, so they run in
	// plain goroutines rather than parallel subtests limited by -test.parallel.
	errs := make([]error, len(records))
	elapsed := make([]time.Duration, len(records))
	var wg sync.WaitGroup
	for i, tc := range records {
		wg.Go(func() {
			start := time.Now()
			errs[i] = VerifyHashedPassword([]byte("synthetic password"), tc.record)
			elapsed[i] = time.Since(start)
		})
	}
	wg.Wait()

	for i, tc := range records {
		require.Error(t, errs[i], tc.name)
		if tc.errContains != "" {
			require.ErrorContains(t, errs[i], tc.errContains, tc.name)
		}
		require.NotContains(t, errs[i].Error(), "Q0FOQVJZ", tc.name)
		require.GreaterOrEqual(t, elapsed[i], DefaultPasswordDelay, "%s: the minimum delay must still apply", tc.name)
		require.Less(t, elapsed[i], DefaultPasswordDelay+time.Second,
			"%s: rejection must not pay a hostile KDF cost", tc.name)
	}

	require.Equal(t, before, argon2idDerivations.Load(), "no hostile record may reach Argon2id")
}

// TestPasswordSecurity_NoCrossFormatReinterpretation verifies that the bytes of
// a valid legacy record placed in the Argon2id layout, and a valid bcrypt record
// of the same password, are never accepted through another format's verifier;
// regression for issue #93.
func TestPasswordSecurity_NoCrossFormatReinterpretation(t *testing.T) {
	t.Parallel()
	const password = "synthetic password"

	legacy, err := newHashedPasswordWithMinIteration([]byte("0123456789abcdef"), []byte(password),
		gutils.HashTypeSha256, MinPasswordHashIteration, MinPasswordHashIteration)
	require.NoError(t, err)
	transplanted := argon2idRecord{params: argon2idParams{memoryKiB: 8192, time: 1, threads: 1},
		salt: legacy.salt, key: legacy.hashedPassword}.String()

	bcryptRecord, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)

	t.Run("legacy digest in argon2id layout", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, VerifyHashedPassword([]byte(password), legacy.String()))
		require.ErrorContains(t, VerifyHashedPassword([]byte(password), transplanted), "password not match")
	})
	t.Run("bcrypt", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, VerifyHashedPassword([]byte(password), string(bcryptRecord)),
			"unsupported password hash format")
	})
}

// TestPasswordSecurity_ConstantTimeTagComparison exercises the tag comparison
// with tags that differ only in their first or only in their last byte; both
// must take the same constant-time mismatch path; regression for issue #93.
func TestPasswordSecurity_ConstantTimeTagComparison(t *testing.T) {
	t.Parallel()
	params := argon2idParams{memoryKiB: 64, time: 1, threads: 1}
	salt := []byte("0123456789abcdef")
	tag, err := deriveArgon2idKey(context.Background(), []byte("synthetic password"), salt, params, 32)
	require.NoError(t, err)

	for _, flip := range []int{0, len(tag) - 1} {
		mutated := bytes.Clone(tag)
		mutated[flip] ^= 0x01
		record := argon2idRecord{params: params, salt: salt, key: mutated}.String()
		err := verifyArgon2idPassword(context.Background(), []byte("synthetic password"), record, argon2idMinSaltLength)
		require.ErrorContains(t, err, "password not match", "flipped byte %d", flip)
	}

	record := argon2idRecord{params: params, salt: salt, key: tag}.String()
	require.NoError(t, verifyArgon2idPassword(context.Background(),
		[]byte("synthetic password"), record, argon2idMinSaltLength))
}

// TestPasswordSecurity_MemoryBudgetGatesDerivation verifies that every Argon2id
// derivation waits on the package memory budget, that out-of-bound parameters
// are refused even by the internal KDF entry point, and that the largest
// accepted record fits the budget; regression for issue #93.
//
// It is deliberately not parallel because it holds the whole shared budget.
func TestPasswordSecurity_MemoryBudgetGatesDerivation(t *testing.T) {
	require.LessOrEqual(t, argon2idMaxMemoryKiB, argon2idMemoryBudgetKiB,
		"a record at the memory cap must be able to acquire the budget")

	params := argon2idParams{memoryKiB: 64, time: 1, threads: 1}
	salt := []byte("0123456789abcdef")
	before := argon2idDerivations.Load()

	_, err := deriveArgon2idKey(context.Background(), []byte("pw"), salt,
		argon2idParams{memoryKiB: argon2idMaxMemoryKiB + 1, time: 1, threads: 1}, 32)
	require.Error(t, err)
	_, err = deriveArgon2idKey(context.Background(), []byte("pw"), salt, params, argon2idMaxKeyLength+1)
	require.Error(t, err)
	require.Equal(t, before, argon2idDerivations.Load())

	require.NoError(t, argon2idMemoryBudget.Acquire(context.Background(), argon2idMemoryBudgetKiB))
	held := true
	defer func() {
		if held {
			argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = deriveArgon2idKey(ctx, []byte("pw"), salt, params, 32)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, before, argon2idDerivations.Load(), "a blocked derivation must not run")

	argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
	held = false

	key, err := deriveArgon2idKey(context.Background(), []byte("pw"), salt, params, 32)
	require.NoError(t, err)
	require.Len(t, key, 32)
	require.Equal(t, before+1, argon2idDerivations.Load())
}
