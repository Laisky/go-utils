package crypto

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/crypto/bcrypt"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
)

// legacyMinPasswordHashIteration keeps compatibility for already stored hashes.
const legacyMinPasswordHashIteration = 1

// HashedPassword is the legacy salted, iterated general-purpose digest record
// "<hasher>.<iterations>.<hexsalt>.<hexhash>".
//
// PasswordHash no longer produces this layout. Stored records keep verifying
// through the legacy branch of VerifyHashedPassword, and PasswordHashNeedsRehash
// always reports them for migration.
//
// Deprecated: new records use Argon2id; see PasswordHash.
type HashedPassword struct {
	salt           []byte
	hasher         gutils.HashTypeInterface
	hashNum        int
	hashedPassword []byte
}

// String serializes the legacy record as "<hasher>.<iterations>.<hexsalt>.<hexhash>",
// which VerifyHashedPassword accepts through its legacy branch.
func (p HashedPassword) String() string {
	return fmt.Sprintf("%s.%d.%s.%s",
		p.hasher.String(),
		p.hashNum,
		hex.EncodeToString(p.salt),
		hex.EncodeToString(p.hashedPassword),
	)
}

// newHashedPasswordWithMinIteration builds a legacy hashed password and validates the iteration range.
// It exists only to verify (and, in tests, to build fixtures for) legacy records.
//
// Params:
//   - salt: random salt bytes appended to rawpassword.
//   - rawpassword: plaintext password bytes.
//   - hasher: hash implementation used to iteratively hash.
//   - hashNum: iteration count for hashing.
//   - minHashNum: minimum accepted iteration count.
//
// Returns:
//   - h: hashed password payload for serialization and verification.
//   - err: wrapped error when iterations are out of range or hashing fails.
func newHashedPasswordWithMinIteration(salt, rawpassword []byte,
	hasher gutils.HashTypeInterface,
	hashNum, minHashNum int) (h HashedPassword, err error) {
	if hashNum < minHashNum || hashNum > MaxPasswordHashIteration {
		return h, errors.Errorf("hashNum %d out of range [%d,%d]",
			hashNum, minHashNum, MaxPasswordHashIteration)
	}

	h.salt = salt
	h.hasher = hasher
	h.hashNum = hashNum

	h.hashedPassword = make([]byte, 0, len(rawpassword)+len(h.salt))
	h.hashedPassword = append(h.hashedPassword, rawpassword...)
	h.hashedPassword = append(h.hashedPassword, h.salt...)
	for i := 0; i < h.hashNum; i++ {
		h.hashedPassword, err = gutils.Hash(h.hasher, bytes.NewReader(h.hashedPassword))
		if err != nil {
			return h, errors.Wrap(err, "calculate password hash")
		}
	}

	return h, nil
}

// parseHashedPassword parses a legacy "<hasher>.<iterations>.<hexsalt>.<hexhash>"
// record and bounds the iteration count. It returns the parsed record, or an
// error when the record does not have four parts, the iteration count is not a
// number within [1, MaxPasswordHashIteration], or the salt or hash is not hex.
func parseHashedPassword(hashedString string) (h HashedPassword, err error) {
	hs := strings.Split(hashedString, ".")
	if len(hs) != 4 {
		return h, errors.Errorf("hashedString must contains 4 parts")
	}

	h.hasher = gutils.HashType(hs[0])
	h.hashNum, err = strconv.Atoi(hs[1])
	if err != nil {
		return h, errors.Wrap(err, "parse hash num")
	}

	if h.hashNum > MaxPasswordHashIteration {
		return h, errors.Errorf("too many iterations %d > %d",
			h.hashNum, MaxPasswordHashIteration)
	} else if h.hashNum <= 0 {
		return h, errors.Errorf("invalid iterations %d", h.hashNum)
	}

	h.salt, err = hex.DecodeString(hs[2])
	if err != nil {
		return h, errors.Wrap(err, "decode salt")
	}

	h.hashedPassword, err = hex.DecodeString(hs[3])
	if err != nil {
		return h, errors.Wrap(err, "decode hashed password")
	}

	return h, nil
}

// isLegacyPasswordHasher reports whether name is a digest that the legacy
// branch can recompute, without constructing a hasher (and so without
// triggering weak-algorithm diagnostics).
func isLegacyPasswordHasher(name string) bool {
	switch gutils.HashType(name) {
	case gutils.HashTypeMD5, gutils.HashTypeSha1, gutils.HashTypeSha256,
		gutils.HashTypeSha512, gutils.HashTypeXxhash:
		return true
	default:
		return false
	}
}

// validateLegacyHashedPassword checks that hashedString is a well-formed legacy
// record with a supported digest name. It returns nil when it is, and an error
// otherwise.
func validateLegacyHashedPassword(hashedString string) error {
	hp, err := parseHashedPassword(hashedString)
	if err != nil {
		return errors.Wrap(err, "parse legacy record")
	}
	if !isLegacyPasswordHasher(hp.hasher.String()) {
		return errors.Errorf("unsupported legacy password digest")
	}

	return nil
}

// verifyLegacyHashedPassword is the documented legacy branch of
// VerifyHashedPassword. It recomputes the iterated digest for rawpassword with
// the stored salt, digest and iteration count, and compares it with the stored
// digest in constant time. Its semantics are unchanged from earlier releases.
// It returns nil on a match, and an error when the record is malformed, the
// digest is unknown or the password differs.
func verifyLegacyHashedPassword(rawpassword []byte, hashedPassword string) error {
	hp, err := parseHashedPassword(hashedPassword)
	if err != nil {
		return errors.Wrap(err, "parse hashed password")
	}

	glog.Shared.Debug("verify legacy iterated-digest password record; it needs rehash",
		zap.Int("hash_num", hp.hashNum),
		zap.Bool("weak_iteration_count", hp.hashNum < MinPasswordHashIteration))

	rawH, err := newHashedPasswordWithMinIteration(
		hp.salt,
		rawpassword,
		hp.hasher,
		hp.hashNum,
		legacyMinPasswordHashIteration,
	)
	if err != nil {
		return errors.Wrap(err, "build hashed password by raw password")
	}

	if subtle.ConstantTimeCompare(hp.hashedPassword, rawH.hashedPassword) != 1 {
		return errors.Errorf("password not match")
	}

	return nil
}

// GeneratePasswordHash generate hashed password by origin password
//
// Deprecated: use PasswordHash instead
func GeneratePasswordHash(password []byte) ([]byte, error) {
	hashed, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Wrap(err, "generate bcrypt password hash")
	}

	return hashed, nil
}

// ValidatePasswordHash validate password is match with hashedPassword
//
// Deprecated: use VerifyHashedPassword instead
func ValidatePasswordHash(hashedPassword, password []byte) bool {
	return bcrypt.CompareHashAndPassword(hashedPassword, password) == nil
}
