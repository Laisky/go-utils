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
// Records explicitly produced by PasswordHashArgon2id use Argon2id (RFC 9106) serialized as a
// PHC string: "$argon2id$v=19$m=<KiB>,t=<passes>,p=<lanes>$<salt>$<hash>".
// Records in the historical "<hasher>.<iterations>.<hexsalt>.<hexhash>"
// iterated-digest layout remain the output of the deprecated PasswordHash API and are verifiable
// through a dedicated legacy branch. The two layouts are told apart by their prefix and are never
// reinterpreted as each other; see password_migration.md for the rollout policy.

const (
	// DefaultPasswordDelay is the minimum wall-clock duration of PasswordHash and
	// PasswordHashArgon2id and VerifyHashedPassword, applied to successful and failed calls.
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
// PasswordHash or PasswordHashArgon2id. It accepts Argon2id PHC records and, through a separate legacy
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

// PasswordHash returns the historical salted, iterated SHA-256 or SHA-512 record
// requested by hasher. Its format, salt lengths, iteration range and minimum
// delay remain compatible with existing writers and readers.
//
// Deprecated: Iterated general-purpose digests are not a modern password KDF.
// Use PasswordHashArgon2id after all readers, including rollback versions, accept
// PHC records. Merely upgrading the library does not opt existing callers in.
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

// PasswordHashArgon2id returns a fresh PHC Argon2id record for password using
// the existing memory budget, 64 MiB, three passes, four lanes, a random 16-byte
// salt and a 32-byte output. Invalid passwords are rejected. Every call takes
// at least DefaultPasswordDelay. VerifyHashedPassword accepts both formats;
// older digest-only readers do not, so adopt this API through a coordinated rollout.
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
// true, call PasswordHashArgon2id only after all active and rollback readers
// support PHC records, then store the new record.
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
