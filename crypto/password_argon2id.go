package crypto

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/crypto/argon2"
	"golang.org/x/sync/semaphore"

	glog "github.com/Laisky/go-utils/v6/log"
)

const (
	// argon2idPHCPrefix is the layout marker of Argon2id PHC records.
	argon2idPHCPrefix = "$argon2id$"
	// argon2idVersionField is the only accepted version field (Argon2 v1.3, 0x13).
	argon2idVersionField = "v=19"
	// argon2idPHCFieldCount is the number of '$'-separated fields, including the
	// empty field before the leading '$'.
	argon2idPHCFieldCount = 6

	// argon2idDefaultMemoryKiB is the memory cost of new records (64 MiB),
	// following the RFC 9106 section 4 second recommended option.
	argon2idDefaultMemoryKiB = 64 * 1024
	// argon2idDefaultTime is the pass count of new records (RFC 9106 option 2).
	argon2idDefaultTime = 3
	// argon2idDefaultThreads is the lane count of new records (RFC 9106 option 2).
	argon2idDefaultThreads = 4
	// argon2idDefaultSaltLength is the random salt length of new records (128 bits).
	argon2idDefaultSaltLength = 16
	// argon2idDefaultKeyLength is the derived tag length of new records (256 bits).
	argon2idDefaultKeyLength = 32

	// argon2idMinMemoryPerThreadKiB is the RFC 9106 minimum memory per lane.
	argon2idMinMemoryPerThreadKiB = 8
	// argon2idMaxMemoryKiB caps the memory a stored record may demand (256 MiB).
	argon2idMaxMemoryKiB = 256 * 1024
	// argon2idMinTime is the minimum accepted pass count.
	argon2idMinTime = 1
	// argon2idMaxTime caps the pass count a stored record may demand.
	argon2idMaxTime = 10
	// argon2idMinThreads is the minimum accepted lane count.
	argon2idMinThreads = 1
	// argon2idMaxThreads caps the lane count (and goroutines) a stored record may demand.
	argon2idMaxThreads = 16
	// argon2idMinSaltLength is the minimum accepted salt length in bytes.
	argon2idMinSaltLength = 16
	// argon2idMaxSaltLength is the maximum accepted salt length in bytes.
	argon2idMaxSaltLength = 64
	// argon2idMinKeyLength is the minimum accepted tag length in bytes.
	argon2idMinKeyLength = 16
	// argon2idMaxKeyLength is the maximum accepted tag length in bytes.
	argon2idMaxKeyLength = 64
	// argon2idMaxDecimalDigits is the digit count of the largest uint32 value.
	argon2idMaxDecimalDigits = 10

	// argon2idMemoryBudgetKiB bounds the Argon2id memory that all concurrent
	// PasswordHash and VerifyHashedPassword calls may hold at once (512 MiB):
	// eight default-cost operations, or two operations at argon2idMaxMemoryKiB.
	argon2idMemoryBudgetKiB = 2 * argon2idMaxMemoryKiB
)

var (
	// defaultArgon2idParams are the cost parameters of new records.
	defaultArgon2idParams = argon2idParams{
		memoryKiB: argon2idDefaultMemoryKiB,
		time:      argon2idDefaultTime,
		threads:   argon2idDefaultThreads,
	}

	// argon2idBase64 is the canonical PHC base64 alphabet without padding.
	argon2idBase64 = base64.RawStdEncoding.Strict()

	// argon2idMemoryBudget is a weighted semaphore, in KiB, that gates every
	// Argon2id derivation so concurrent calls cannot multiply memory without limit.
	argon2idMemoryBudget = semaphore.NewWeighted(argon2idMemoryBudgetKiB)

	// argon2idDerivations counts Argon2id derivations; tests use it to prove
	// that rejected records never reach the KDF.
	argon2idDerivations atomic.Uint64
)

// argon2idParams holds validated Argon2id cost parameters.
type argon2idParams struct {
	memoryKiB uint32
	time      uint32
	threads   uint8
}

// validate checks that the parameters lie within the accepted bounds. It
// returns nil when they do, and an error naming the violated bound otherwise.
func (p argon2idParams) validate() error {
	switch {
	case p.threads < argon2idMinThreads || p.threads > argon2idMaxThreads:
		return errors.Errorf("argon2id parallelism must be within [%d,%d]",
			argon2idMinThreads, argon2idMaxThreads)
	case p.time < argon2idMinTime || p.time > argon2idMaxTime:
		return errors.Errorf("argon2id time cost must be within [%d,%d]",
			argon2idMinTime, argon2idMaxTime)
	case p.memoryKiB < argon2idMinMemoryPerThreadKiB*uint32(p.threads) || p.memoryKiB > argon2idMaxMemoryKiB:
		return errors.Errorf("argon2id memory cost must be within [%d*p,%d] KiB",
			argon2idMinMemoryPerThreadKiB, argon2idMaxMemoryKiB)
	default:
		return nil
	}
}

// argon2idRecord is a parsed, bounds-checked Argon2id PHC record.
type argon2idRecord struct {
	params argon2idParams
	salt   []byte
	key    []byte
}

// weakerThanDefaults reports whether the record's memory cost, pass count,
// salt length or tag length is below the defaults used for new records.
func (r argon2idRecord) weakerThanDefaults() bool {
	return r.params.memoryKiB < defaultArgon2idParams.memoryKiB ||
		r.params.time < defaultArgon2idParams.time ||
		len(r.salt) < argon2idDefaultSaltLength ||
		len(r.key) < argon2idDefaultKeyLength
}

// String serializes the record as a canonical PHC string
// "$argon2id$v=19$m=<KiB>,t=<n>,p=<n>$<salt>$<hash>" with unpadded base64.
func (r argon2idRecord) String() string {
	var b strings.Builder
	b.WriteString(argon2idPHCPrefix)
	b.WriteString(argon2idVersionField)
	b.WriteString("$m=")
	b.WriteString(strconv.FormatUint(uint64(r.params.memoryKiB), 10))
	b.WriteString(",t=")
	b.WriteString(strconv.FormatUint(uint64(r.params.time), 10))
	b.WriteString(",p=")
	b.WriteString(strconv.FormatUint(uint64(r.params.threads), 10))
	b.WriteByte('$')
	b.WriteString(argon2idBase64.EncodeToString(r.salt))
	b.WriteByte('$')
	b.WriteString(argon2idBase64.EncodeToString(r.key))
	return b.String()
}

// parseArgon2idRecord strictly parses encoded as an Argon2id PHC record and
// checks every bound before any key derivation can start. minSaltLength is the
// minimum accepted salt length in bytes; production callers pass
// argon2idMinSaltLength. It returns the parsed record, or an error that never
// echoes the input when the record is oversized, has missing or extra fields,
// an unsupported version, non-canonical numbers or base64, or out-of-range values.
func parseArgon2idRecord(encoded string, minSaltLength int) (rec argon2idRecord, err error) {
	if len(encoded) > MaxHashedPasswordLength {
		return rec, errors.Errorf("argon2id record is too long")
	}

	fields := strings.SplitN(encoded, "$", argon2idPHCFieldCount+1)
	if len(fields) != argon2idPHCFieldCount || fields[0] != "" || fields[1] != "argon2id" {
		return rec, errors.Errorf("argon2id record must have exactly %d '$'-separated fields",
			argon2idPHCFieldCount-1)
	}
	if fields[2] != argon2idVersionField {
		return rec, errors.Errorf("unsupported argon2id version, want %s", argon2idVersionField)
	}

	if rec.params, err = parseArgon2idParams(fields[3]); err != nil {
		return rec, errors.Wrap(err, "parse argon2id parameters")
	}
	if rec.salt, err = decodeArgon2idBase64(fields[4], minSaltLength, argon2idMaxSaltLength); err != nil {
		return rec, errors.Wrap(err, "decode argon2id salt")
	}
	if rec.key, err = decodeArgon2idBase64(fields[5], argon2idMinKeyLength, argon2idMaxKeyLength); err != nil {
		return rec, errors.Wrap(err, "decode argon2id hash")
	}

	return rec, nil
}

// parseArgon2idParams parses the "m=<KiB>,t=<n>,p=<n>" field in exactly that
// order and validates the values. It returns the parameters, or an error when
// the field has missing, extra or reordered entries, or out-of-range values.
func parseArgon2idParams(field string) (params argon2idParams, err error) {
	entries := strings.SplitN(field, ",", 4)
	if len(entries) != 3 {
		return params, errors.Errorf("argon2id parameters must be exactly m,t,p")
	}

	memoryKiB, err := parseArgon2idDecimal(entries[0], "m=")
	if err != nil {
		return params, errors.Wrap(err, "memory cost")
	}
	passes, err := parseArgon2idDecimal(entries[1], "t=")
	if err != nil {
		return params, errors.Wrap(err, "time cost")
	}
	lanes, err := parseArgon2idDecimal(entries[2], "p=")
	if err != nil {
		return params, errors.Wrap(err, "parallelism")
	}
	if lanes < argon2idMinThreads || lanes > argon2idMaxThreads {
		return params, errors.Errorf("argon2id parallelism must be within [%d,%d]",
			argon2idMinThreads, argon2idMaxThreads)
	}

	params = argon2idParams{memoryKiB: memoryKiB, time: passes, threads: uint8(lanes)}
	if err = params.validate(); err != nil {
		return params, errors.Wrap(err, "validate argon2id parameters")
	}

	return params, nil
}

// parseArgon2idDecimal parses entry as "<key><digits>" where digits is a
// canonical positive decimal (no sign, no leading zero) that fits in uint32.
// It returns the value, or an error that names only key when entry is malformed.
func parseArgon2idDecimal(entry, key string) (uint32, error) {
	digits, ok := strings.CutPrefix(entry, key)
	if !ok {
		return 0, errors.Errorf("expected %q entry", key)
	}
	if len(digits) == 0 || len(digits) > argon2idMaxDecimalDigits || digits[0] == '0' {
		return 0, errors.Errorf("%q must be a canonical positive decimal", key)
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, errors.Errorf("%q must be a canonical positive decimal", key)
		}
	}

	// digits is 1-10 ASCII digits, so the only possible failure is uint32 overflow;
	// the strconv error is not wrapped because it would echo the input.
	value, err := strconv.ParseUint(digits, 10, 32)
	if err != nil {
		return 0, errors.Errorf("%q exceeds uint32", key)
	}

	return uint32(value), nil
}

// decodeArgon2idBase64 decodes field as canonical unpadded standard base64
// whose decoded length lies within [minLen, maxLen]. The encoded length is
// checked before decoding, and the result must re-encode to field exactly, which
// rejects padding, line breaks and non-zero trailing bits. It returns the
// decoded bytes, or an error that never echoes field.
func decodeArgon2idBase64(field string, minLen, maxLen int) ([]byte, error) {
	if len(field) < argon2idBase64.EncodedLen(minLen) || len(field) > argon2idBase64.EncodedLen(maxLen) {
		return nil, errors.Errorf("encoded length must cover [%d,%d] bytes", minLen, maxLen)
	}

	decoded, err := argon2idBase64.DecodeString(field)
	if err != nil {
		return nil, errors.Wrap(err, "invalid unpadded base64")
	}
	if len(decoded) < minLen || len(decoded) > maxLen {
		return nil, errors.Errorf("decoded length must be within [%d,%d] bytes", minLen, maxLen)
	}
	if argon2idBase64.EncodeToString(decoded) != field {
		return nil, errors.Errorf("base64 is not canonical")
	}

	return decoded, nil
}

// deriveArgon2idKey runs Argon2id over password and salt with params and
// returns keyLen bytes. It re-validates params and keyLen, then waits on the
// package memory budget so concurrent derivations stay within
// argon2idMemoryBudgetKiB. It returns an error when the parameters are out of
// bounds or ctx ends before the budget becomes available.
func deriveArgon2idKey(ctx context.Context,
	password, salt []byte, params argon2idParams, keyLen int) ([]byte, error) {
	if err := params.validate(); err != nil {
		return nil, errors.Wrap(err, "validate argon2id parameters")
	}
	if keyLen < argon2idMinKeyLength || keyLen > argon2idMaxKeyLength {
		return nil, errors.Errorf("argon2id key length must be within [%d,%d]",
			argon2idMinKeyLength, argon2idMaxKeyLength)
	}

	weight := int64(params.memoryKiB)
	if err := argon2idMemoryBudget.Acquire(ctx, weight); err != nil {
		return nil, errors.Wrap(err, "wait for argon2id memory budget")
	}
	defer argon2idMemoryBudget.Release(weight)

	argon2idDerivations.Add(1)
	glog.Shared.Debug("derive argon2id password key",
		zap.Uint32("memory_kib", params.memoryKiB),
		zap.Uint32("time", params.time),
		zap.Uint8("threads", params.threads))

	return argon2.IDKey(password, salt, params.time, params.memoryKiB, params.threads, uint32(keyLen)), nil
}

// newArgon2idPasswordRecord derives a new record for password with params, a
// fresh argon2idDefaultSaltLength-byte salt and an argon2idDefaultKeyLength-byte
// tag. It returns the PHC string, or an error when salt generation or
// derivation fails.
func newArgon2idPasswordRecord(ctx context.Context, password []byte, params argon2idParams) (string, error) {
	salt, err := Salt(argon2idDefaultSaltLength)
	if err != nil {
		return "", errors.Wrap(err, "generate salt")
	}

	key, err := deriveArgon2idKey(ctx, password, salt, params, argon2idDefaultKeyLength)
	if err != nil {
		return "", errors.Wrap(err, "derive argon2id key")
	}

	return argon2idRecord{params: params, salt: salt, key: key}.String(), nil
}

// verifyArgon2idPassword parses encoded strictly (with minSaltLength as the
// salt floor), derives a tag of the stored length from password, and compares
// it with the stored tag in constant time. It returns nil on a match, and an
// error when the record is invalid, derivation fails or the password differs.
func verifyArgon2idPassword(ctx context.Context, password []byte, encoded string, minSaltLength int) error {
	rec, err := parseArgon2idRecord(encoded, minSaltLength)
	if err != nil {
		return errors.Wrap(err, "parse hashed password")
	}

	derived, err := deriveArgon2idKey(ctx, password, rec.salt, rec.params, len(rec.key))
	if err != nil {
		return errors.Wrap(err, "derive argon2id key")
	}

	if subtle.ConstantTimeCompare(rec.key, derived) != 1 {
		return errors.Errorf("password not match")
	}

	return nil
}
