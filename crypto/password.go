package crypto

import (
	"context"
	"crypto/rand"
	"math/big"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	gutils "github.com/Laisky/go-utils/v6"
)

// Password records
//
// Records produced by the opt-in PasswordHashArgon2id use Argon2id (RFC 9106) serialized as a
// PHC string: "$argon2id$v=19$m=<KiB>,t=<passes>,p=<lanes>$<salt>$<hash>".
// Records in the historical "<hasher>.<iterations>.<hexsalt>.<hexhash>"
// iterated-digest layout remain verifiable through a dedicated legacy branch
// only. PasswordHash retains the historical writer. The two layouts are told apart by their prefix and are never
// reinterpreted as each other; see password_migration.md for the rollout policy.

const (
	// DefaultPasswordDelay is the minimum wall-clock duration of PasswordHash and
	// VerifyHashedPassword, applied to both successful and failed calls.
	DefaultPasswordDelay = 2 * time.Second
	// MaxPasswordHashIteration limits the iteration count of legacy records.
	MaxPasswordHashIteration = 1000000
	// MaxHashedPasswordLength limits serialized password hash input to prevent parse-time DoS.
	MaxHashedPasswordLength = 4096
	// MaxPasswordLength limits the plaintext password length to prevent DoS.
	MaxPasswordLength = 1024
	// MinPasswordHashIteration is the minimum iteration count historically used
	// for new legacy records. Legacy records below it still verify; every legacy
	// record, whatever its iteration count, reports PasswordHashNeedsRehash.
	MinPasswordHashIteration = 10000
)

// passwordRecordFormat identifies the family of a serialized password record.
type passwordRecordFormat int

const (
	// passwordRecordUnsupported marks a record that matches no accepted layout.
	passwordRecordUnsupported passwordRecordFormat = iota
	// passwordRecordArgon2id marks a PHC "$argon2id$" record.
	passwordRecordArgon2id
	// passwordRecordLegacy marks a "<hasher>.<n>.<salt>.<hash>" iterated-digest record.
	passwordRecordLegacy
)

// detectPasswordRecordFormat classifies hashedPassword strictly by its layout
// marker. It returns passwordRecordArgon2id for the "$argon2id$" prefix,
// passwordRecordUnsupported for any other record containing '$' (for example
// "$argon2i$", "$scrypt$" or bcrypt "$2a$"), and passwordRecordLegacy otherwise.
// The legacy branch still has to pass the strict legacy parser.
func detectPasswordRecordFormat(hashedPassword string) passwordRecordFormat {
	switch {
	case strings.HasPrefix(hashedPassword, argon2idPHCPrefix):
		return passwordRecordArgon2id
	case strings.Contains(hashedPassword, "$"):
		return passwordRecordUnsupported
	default:
		return passwordRecordLegacy
	}
}

// VerifyHashedPassword checks rawpassword against a record produced by
// PasswordHash. It accepts Argon2id PHC records and, through a separate legacy
// branch, historical iterated-digest records; any other layout is rejected.
// Every call takes at least DefaultPasswordDelay. It returns nil when the
// password matches, and an error that never echoes the password or the stored
// hash when the inputs are empty or oversized, the record is malformed or out
// of bounds, or the password does not match.
func VerifyHashedPassword(rawpassword []byte, hashedPassword string) (err error) {
	defer gutils.NewDelay(DefaultPasswordDelay).Wait()

	if len(rawpassword) == 0 || len(hashedPassword) == 0 {
		return errors.Errorf("rawpassword or hashedPassword is empty")
	} else if len(rawpassword) > MaxPasswordLength {
		return errors.Errorf("password is too long")
	} else if len(hashedPassword) > MaxHashedPasswordLength {
		return errors.Errorf("hashedPassword is too long")
	}

	switch detectPasswordRecordFormat(hashedPassword) {
	case passwordRecordArgon2id:
		return verifyArgon2idPassword(context.Background(), rawpassword, hashedPassword, argon2idMinSaltLength)
	case passwordRecordLegacy:
		return verifyLegacyHashedPassword(rawpassword, hashedPassword)
	default:
		return errors.Errorf("unsupported password hash format")
	}
}

// PasswordHash produces the historical salted, iterated SHA-256 or SHA-512 record
// selected by hasher. Salt sizes, iteration bounds, delay and wire format remain
// compatible with old readers, including during rolling upgrades and rollback.
//
// Deprecated: General-purpose digests are not memory-hard password KDFs. Use
// PasswordHashArgon2id only after all readers and rollback versions support PHC records.
func PasswordHash(password []byte, hasher gutils.HashType) (hashedPassword string, err error) {
	defer gutils.NewDelay(DefaultPasswordDelay).Wait()

	if len(password) == 0 {
		return "", errors.Errorf("password is empty")
	} else if len(password) > MaxPasswordLength {
		return "", errors.Errorf("password is too long")
	}

	var salt []byte
	switch hasher {
	case gutils.HashTypeSha256:
		if salt, err = Salt(256); err != nil {
			return "", errors.Wrap(err, "generate salt")
		}
	case gutils.HashTypeSha512:
		if salt, err = Salt(512); err != nil {
			return "", errors.Wrap(err, "generate salt")
		}
	default:
		return "", errors.Errorf("only supprt sha256,sha512")
	}

	n, err := rand.Int(rand.Reader, big.NewInt(MaxPasswordHashIteration-MinPasswordHashIteration))
	if err != nil {
		return "", errors.Wrap(err, "generate hash count")
	}
	hashNum := int(n.Int64()) + MinPasswordHashIteration

	h, err := newHashedPasswordWithMinIteration(salt, password, hasher, hashNum, MinPasswordHashIteration)
	if err != nil {
		return "", errors.Wrap(err, "hashing password")
	}

	return h.String(), nil
}

// PasswordHashArgon2id returns an explicitly selected Argon2id PHC record with a fresh
// 16-byte salt and the default bounded memory-hard parameters. It rejects empty or
// oversized passwords, uses shared memory admission and preserves DefaultPasswordDelay.
// Verify with VerifyHashedPassword. Old digest-only readers cannot read this format;
// upgrading the library alone does not opt existing PasswordHash callers into it.
func PasswordHashArgon2id(password []byte) (string, error) {
	defer gutils.NewDelay(DefaultPasswordDelay).Wait()
	if len(password) == 0 {
		return "", errors.New("password is empty")
	}
	if len(password) > MaxPasswordLength {
		return "", errors.New("password is too long")
	}
	record, err := newArgon2idPasswordRecord(context.Background(), password, defaultArgon2idParams)
	if err != nil {
		return "", errors.Wrap(err, "hashing password")
	}
	return record, nil
}

// PasswordHashNeedsRehash reports whether a stored record should be replaced by
// a fresh PasswordHashArgon2id record after the next successful VerifyHashedPassword.
// It returns true for every well-formed legacy iterated-digest record and for
// Argon2id records whose memory cost, pass count, salt length or output length
// is below the current defaults. Parallelism is not compared because, for fixed
// memory and passes, fewer lanes do not reduce an attacker's cost. It performs
// no key derivation and does not need the password. It returns an error, which
// never echoes the input, for empty, oversized, malformed or unsupported records.
//
// Rollout: call VerifyHashedPassword; when it succeeds and this function returns
// true, opt into PasswordHashArgon2id only after every reader and rollback version
// supports that format. PasswordHash itself does not migrate record formats.
func PasswordHashNeedsRehash(hashedPassword string) (bool, error) {
	if len(hashedPassword) == 0 {
		return false, errors.Errorf("hashedPassword is empty")
	} else if len(hashedPassword) > MaxHashedPasswordLength {
		return false, errors.Errorf("hashedPassword is too long")
	}

	switch detectPasswordRecordFormat(hashedPassword) {
	case passwordRecordArgon2id:
		rec, parseErr := parseArgon2idRecord(hashedPassword, argon2idMinSaltLength)
		if parseErr != nil {
			return false, errors.Wrap(parseErr, "parse hashed password")
		}
		return rec.weakerThanDefaults(), nil
	case passwordRecordLegacy:
		if legacyErr := validateLegacyHashedPassword(hashedPassword); legacyErr != nil {
			return false, errors.Wrap(legacyErr, "parse hashed password")
		}
		return true, nil
	default:
		return false, errors.Errorf("unsupported password hash format")
	}
}
