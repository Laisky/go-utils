package crypto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"

	gutils "github.com/Laisky/go-utils/v6"
)

const (
	// argon2idReferenceVector is the argon2id known-answer test from the
	// reference implementation (phc-winner-argon2 src/test.c): v=19, t=2,
	// m=2^16 KiB, p=1, password "password", salt "somesalt". It was cross-checked
	// with argon2-cffi 25.1.0.
	argon2idReferenceVector = "$argon2id$v=19$m=65536,t=2,p=1$c29tZXNhbHQ$CTFhFdXPJO1aFaMaO6Mm5c8y7cJHAph8ArZWb2GRPPc"
	// argon2idReferenceTagHex is the raw tag of argon2idReferenceVector as printed by src/test.c.
	argon2idReferenceTagHex = "09316115d5cf24ed5a15a31a3ba326e5cf32edc24702987c02b6566f61913cf7"
	// argon2idFixturePassword is the public synthetic password of the policy fixtures.
	argon2idFixturePassword = "correct horse battery staple"
	// argon2idPolicyVector was produced independently by argon2-cffi 25.1.0 with
	// the default parameters (m=65536, t=3, p=4, 32-byte tag) and the 16-byte
	// salt "go-utils#93-salt".
	argon2idPolicyVector = "$argon2id$v=19$m=65536,t=3,p=4$Z28tdXRpbHMjOTMtc2FsdA$" +
		"eFDBjf8sQd/2ZV+KOVzHuBgHJ0eI2snK+vlXW7VPKag"
	// argon2idCheapVector was produced independently by argon2-cffi 25.1.0 with
	// weak-but-accepted parameters (m=8192, t=1, p=2) and the same salt.
	argon2idCheapVector = "$argon2id$v=19$m=8192,t=1,p=2$Z28tdXRpbHMjOTMtc2FsdA$" +
		"3TrBFGOYTz4ZCTT5h8VcEAHgml13Xb3BSoJ/bYjemhs"
)

// legacyPasswordRecordPattern matches the legacy iterated-digest record layout
// "<hasher>.<iterations>.<hexsalt>.<hexhash>".
var legacyPasswordRecordPattern = regexp.MustCompile(`^[a-z0-9]+\.[0-9]+\.[0-9a-f]*\.[0-9a-f]+$`)

// newLegacyPasswordFixture builds a legacy record for password with the
// existing legacy builder and returns its serialized form; it fails t on error.
func newLegacyPasswordFixture(t *testing.T, password string, hasher gutils.HashType, iterations int) string {
	t.Helper()
	hp, err := newHashedPasswordWithMinIteration([]byte("legacy-salt"), []byte(password),
		hasher, iterations, legacyMinPasswordHashIteration)
	require.NoError(t, err)
	return hp.String()
}

// TestPasswordSecurity_NewRecordsUseArgon2id verifies that PasswordHash emits a
// PHC-formatted Argon2id record with the default parameters and a fresh salt
// instead of an iterated general-purpose digest, regardless of the retained
// hasher argument, and that the record round-trips and needs no rehash;
// regression for issue #93.
func TestPasswordSecurity_NewRecordsUseArgon2id(t *testing.T) {
	t.Parallel()
	hashers := []gutils.HashType{gutils.HashTypeSha256, gutils.HashTypeSha512}
	records := make([]string, len(hashers))
	errs := make([]error, len(hashers))
	var wg sync.WaitGroup
	for i, hasher := range hashers {
		wg.Go(func() { records[i], errs[i] = PasswordHash([]byte("synthetic password"), hasher) })
	}
	wg.Wait()

	for i, hasher := range hashers {
		require.NoError(t, errs[i], hasher)
		hashed := records[i]
		require.False(t, legacyPasswordRecordPattern.MatchString(hashed),
			"new record uses the legacy iterated %s digest format (not a password KDF)", hasher)
		require.True(t, strings.HasPrefix(hashed, "$argon2id$v=19$m=65536,t=3,p=4$"),
			"new record must be an Argon2id PHC string with the default parameters")

		rec, err := parseArgon2idRecord(hashed, argon2idMinSaltLength)
		require.NoError(t, err)
		require.Equal(t, defaultArgon2idParams, rec.params)
		require.Len(t, rec.salt, argon2idDefaultSaltLength)
		require.Len(t, rec.key, argon2idDefaultKeyLength)

		needsRehash, err := PasswordHashNeedsRehash(hashed)
		require.NoError(t, err)
		require.False(t, needsRehash)
	}
	require.NotEqual(t, records[0], records[1], "every record must use a fresh salt")

	// Default-cost Argon2id runs are slow under -race, so only one record
	// repeats the round trip; TestPasswordSecurity_IndependentVectors and
	// TestPasswordSecurity_ConcurrentOperations cover wrong passwords.
	require.NoError(t, VerifyHashedPassword([]byte("synthetic password"), records[0]))
}

// TestPasswordSecurity_Argon2idReferenceVector checks the PHC parser and
// encoder, the KDF core and the verifier against the reference implementation's
// argon2id known-answer test, and pins that the strict public policy rejects
// its 8-byte salt; regression for issue #93.
func TestPasswordSecurity_Argon2idReferenceVector(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0x13, argon2.Version, "argon2idVersionField assumes Argon2 v1.3")

	wantTag, err := hex.DecodeString(argon2idReferenceTagHex)
	require.NoError(t, err)
	params := argon2idParams{memoryKiB: 65536, time: 2, threads: 1}
	rec := argon2idRecord{params: params, salt: []byte("somesalt"), key: wantTag}
	require.Equal(t, argon2idReferenceVector, rec.String())

	parsed, err := parseArgon2idRecord(argon2idReferenceVector, len("somesalt"))
	require.NoError(t, err)
	require.Equal(t, rec, parsed)

	// verifyArgon2idPassword runs deriveArgon2idKey and compares with the
	// reference tag, so the KDF core is checked against src/test.c here.
	var matchErr, mismatchErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		matchErr = verifyArgon2idPassword(context.Background(),
			[]byte("password"), argon2idReferenceVector, len("somesalt"))
	})
	wg.Go(func() {
		mismatchErr = verifyArgon2idPassword(context.Background(),
			[]byte("Password"), argon2idReferenceVector, len("somesalt"))
	})
	wg.Wait()
	require.NoError(t, matchErr)
	require.ErrorContains(t, mismatchErr, "password not match")

	_, err = parseArgon2idRecord(argon2idReferenceVector, argon2idMinSaltLength)
	require.ErrorContains(t, err, "salt")
	_, err = PasswordHashNeedsRehash(argon2idReferenceVector)
	require.ErrorContains(t, err, "salt")
}

// TestPasswordSecurity_IndependentVectors verifies records produced by an
// independent Argon2id implementation through the public API, including the
// rehash signal for accepted-but-weak parameters; regression for issue #93.
func TestPasswordSecurity_IndependentVectors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		vector      string
		needsRehash bool
	}{
		{name: "default parameters", vector: argon2idPolicyVector, needsRehash: false},
		{name: "weak parameters", vector: argon2idCheapVector, needsRehash: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, VerifyHashedPassword([]byte(argon2idFixturePassword), tc.vector))
			if tc.needsRehash { // the cheap vector also covers a near-miss password
				err := VerifyHashedPassword([]byte("CANARY correct horse battery staple"), tc.vector)
				require.ErrorContains(t, err, "password not match")
				require.NotContains(t, err.Error(), "CANARY", "errors must not echo the password")
			}

			needsRehash, err := PasswordHashNeedsRehash(tc.vector)
			require.NoError(t, err)
			require.Equal(t, tc.needsRehash, needsRehash)
		})
	}
}

// TestPasswordSecurity_LegacyRecordsStillVerify verifies that legacy
// iterated-digest records keep verifying through the legacy branch, reject
// wrong passwords, and always report needs-rehash; regression for issue #93.
func TestPasswordSecurity_LegacyRecordsStillVerify(t *testing.T) {
	t.Parallel()

	// independent is computed here with crypto/sha256 rather than the legacy builder.
	digest := []byte("legacy passwordindependent-salt")
	for range 3 {
		sum := sha256.Sum256(digest)
		digest = sum[:]
	}
	independent := fmt.Sprintf("sha256.3.%s.%s", hex.EncodeToString([]byte("independent-salt")),
		hex.EncodeToString(digest))

	for name, record := range map[string]string{
		"independent sha256":     independent,
		"sha256 policy":          newLegacyPasswordFixture(t, "legacy password", gutils.HashTypeSha256, MinPasswordHashIteration),
		"sha512 policy":          newLegacyPasswordFixture(t, "legacy password", gutils.HashTypeSha512, MinPasswordHashIteration),
		"sha256 weak iterations": newLegacyPasswordFixture(t, "legacy password", gutils.HashTypeSha256, 5),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, VerifyHashedPassword([]byte("legacy password"), record))
			require.ErrorContains(t, VerifyHashedPassword([]byte("legacy passworD"), record), "password not match")

			needsRehash, err := PasswordHashNeedsRehash(record)
			require.NoError(t, err)
			require.True(t, needsRehash)
		})
	}
}

// TestPasswordSecurity_NeedsRehash pins the migration signal for current,
// stronger, weaker, legacy and invalid records; regression for issue #93.
func TestPasswordSecurity_NeedsRehash(t *testing.T) {
	t.Parallel()
	salt16, salt32 := []byte("0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef")
	key16, key32, key64 := make([]byte, 16), make([]byte, 32), make([]byte, 64)
	record := func(m, tc uint32, p uint8, salt, key []byte) string {
		return argon2idRecord{params: argon2idParams{memoryKiB: m, time: tc, threads: p}, salt: salt, key: key}.String()
	}

	for _, tc := range []struct {
		name   string
		record string
		want   bool
	}{
		{name: "defaults", record: record(65536, 3, 4, salt16, key32), want: false},
		{name: "stronger", record: record(131072, 4, 8, salt32, key64), want: false},
		{name: "fewer lanes", record: record(65536, 3, 1, salt16, key32), want: false},
		{name: "less memory", record: record(32768, 3, 4, salt16, key32), want: true},
		{name: "fewer passes", record: record(65536, 2, 4, salt16, key32), want: true},
		{name: "shorter tag", record: record(65536, 3, 4, salt16, key16), want: true},
		{name: "legacy md5", record: "md5.3..00", want: true},
		{name: "legacy xxhash", record: "xxhash.10000.00.00", want: true},
	} {
		got, err := PasswordHashNeedsRehash(tc.record)
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.want, got, tc.name)
	}

	for name, invalid := range map[string]string{
		"empty":                 "",
		"oversized":             strings.Repeat("a", MaxHashedPasswordLength+1),
		"argon2id bad version":  strings.Replace(argon2idPolicyVector, "v=19", "v=16", 1),
		"argon2i":               strings.Replace(argon2idPolicyVector, "$argon2id$", "$argon2i$", 1),
		"bcrypt":                "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		"legacy unknown digest": "CANARY.3..00",
		"legacy zero iteration": "sha256.0..00",
		"legacy three parts":    "sha256.3.00",
	} {
		_, err := PasswordHashNeedsRehash(invalid)
		require.Error(t, err, name)
		require.NotContains(t, err.Error(), "CANARY", name)
	}
}

// TestPasswordSecurity_ConcurrentOperations runs Argon2id and legacy
// verifications in parallel so the race detector and the memory budget are
// exercised together (TestPasswordSecurity_NewRecordsUseArgon2id covers
// concurrent generation); regression for issue #93.
func TestPasswordSecurity_ConcurrentOperations(t *testing.T) {
	t.Parallel()
	legacy := newLegacyPasswordFixture(t, argon2idFixturePassword, gutils.HashTypeSha256, MinPasswordHashIteration)

	type job struct {
		record   string
		password string
		match    bool
	}
	var jobs []job
	for range 3 {
		jobs = append(jobs,
			job{record: argon2idCheapVector, password: argon2idFixturePassword, match: true},
			job{record: argon2idCheapVector, password: "wrong", match: false},
			job{record: legacy, password: argon2idFixturePassword, match: true},
			job{record: legacy, password: "wrong", match: false},
		)
	}

	verifyErrs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Go(func() { verifyErrs[i] = VerifyHashedPassword([]byte(j.password), j.record) })
	}
	wg.Wait()

	for i, j := range jobs {
		if j.match {
			require.NoError(t, verifyErrs[i], "job %d", i)
		} else {
			require.ErrorContains(t, verifyErrs[i], "password not match", "job %d", i)
		}
	}
}
