package crypto

import (
	"context"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	gutils "github.com/Laisky/go-utils/v6"
)

// Password records
//
// New records produced by PasswordHash use Argon2id (RFC 9106) serialized as a
// PHC string: "$argon2id$v=19$m=<KiB>,t=<passes>,p=<lanes>$<salt>$<hash>".
// Records in the historical "<hasher>.<iterations>.<hexsalt>.<hexhash>"
// iterated-digest layout remain verifiable through a dedicated legacy branch
// only. The two layouts are told apart by their prefix and are never
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

// PasswordHash returns a new Argon2id PHC record for password that can be
// checked by VerifyHashedPassword. New records use a 16-byte random salt, a
// 32-byte output and the RFC 9106 second recommended parameter set (m=64 MiB,
// t=3, p=4). The hasher argument is retained only for API compatibility: it is
// still validated (only gutils.HashTypeSha256 and gutils.HashTypeSha512 are
// accepted) but otherwise ignored, so no general-purpose digest is applied to
// the password. Every call takes at least DefaultPasswordDelay. It returns the
// serialized record, or an error when the password is empty or too long, the
// hasher is unsupported, or salt generation fails.
func PasswordHash(password []byte, hasher gutils.HashType) (hashedPassword string, err error) {
	defer gutils.NewDelay(DefaultPasswordDelay).Wait()

	if len(password) == 0 {
		return "", errors.Errorf("password is empty")
	} else if len(password) > MaxPasswordLength {
		return "", errors.Errorf("password is too long")
	}

	switch hasher {
	case gutils.HashTypeSha256, gutils.HashTypeSha512:
	default:
		return "", errors.Errorf("only supprt sha256,sha512")
	}

	hashedPassword, err = newArgon2idPasswordRecord(context.Background(), password, defaultArgon2idParams)
	if err != nil {
		return "", errors.Wrap(err, "hashing password")
	}

	return hashedPassword, nil
}

// PasswordHashNeedsRehash reports whether a stored record should be replaced by
// a fresh PasswordHash record after the next successful VerifyHashedPassword.
// It returns true for every well-formed legacy iterated-digest record and for
// Argon2id records whose memory cost, pass count, salt length or output length
// is below the current defaults. Parallelism is not compared because, for fixed
// memory and passes, fewer lanes do not reduce an attacker's cost. It performs
// no key derivation and does not need the password. It returns an error, which
// never echoes the input, for empty, oversized, malformed or unsupported records.
//
// Rollout: call VerifyHashedPassword; when it succeeds and this function returns
// true, call PasswordHash with the same plaintext and store the new record.
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
