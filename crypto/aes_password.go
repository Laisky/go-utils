package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"golang.org/x/crypto/argon2"
)

// Password-based encryption format (version 1):
//
//	header     = magic "GUPWAEAD" (8) || version 0x01 (1) || KDF id 0x01 Argon2id (1) ||
//	             time uint32 BE (4) || memory KiB uint32 BE (4) || parallelism (1) || salt (16)
//	key        = Argon2id(password, salt, time, memory, parallelism, 32 bytes)
//	ciphertext = header || nonce (12) || AES-256-GCM(key, nonce, plaintext, AAD=header)
//
// The header is authenticated as GCM additional data, and it also determines the
// derived key, so tampering with any header field makes decryption fail.
const (
	passwordFormatMagic    = "GUPWAEAD"
	passwordFormatVersion1 = byte(1)
	passwordKDFArgon2id    = byte(1)
	passwordSaltLen        = 16
	passwordKeyLen         = 32
	passwordHeaderLen      = len(passwordFormatMagic) + 1 + 1 + 4 + 4 + 1 + passwordSaltLen
	passwordOverhead       = passwordHeaderLen + AesGcmIvLen + AesGcmTagLen

	// passwordKDFMaxTime, passwordKDFMaxMemoryKiB and passwordKDFMaxParallelism
	// bound untrusted header parameters before any Argon2id work starts.
	passwordKDFMaxTime        = 10
	passwordKDFMaxMemoryKiB   = 256 * 1024
	passwordKDFMaxParallelism = 16

	// passwordKDFMinTime and passwordKDFMinMemoryKiB are the encryption floor for
	// caller-selected parameters (OWASP Argon2id baseline: m=19 MiB, t=2).
	passwordKDFMinTime      = 2
	passwordKDFMinMemoryKiB = 19 * 1024

	// passwordEncryptorMaxMessages bounds random 96-bit nonces under one derived
	// key, following the NIST SP 800-38D limit of 2^32 invocations.
	passwordEncryptorMaxMessages = uint64(1) << 32
)

// PasswordKDFParams are the Argon2id cost parameters of the password format.
type PasswordKDFParams struct {
	// Time is the number of Argon2id passes.
	Time uint32
	// MemoryKiB is the Argon2id memory cost in KiB.
	MemoryKiB uint32
	// Parallelism is the number of Argon2id lanes.
	Parallelism uint8
}

// DefaultPasswordKDFParams returns the default Argon2id parameters, the second
// recommended option of RFC 9106: t=3 passes, m=64 MiB, p=4 lanes.
func DefaultPasswordKDFParams() PasswordKDFParams {
	return PasswordKDFParams{Time: 3, MemoryKiB: 64 * 1024, Parallelism: 4}
}

// validateForDecrypt checks untrusted parameters against the hard resource
// bounds (t <= 10, m <= 256 MiB, 1 <= p <= 16, m >= 8*p). It returns an error
// for any out-of-range field and is cheap enough to run before Argon2id.
func (p PasswordKDFParams) validateForDecrypt() error {
	if p.Time < 1 || p.Time > passwordKDFMaxTime {
		return errors.Errorf("argon2id time %d out of range [1, %d]", p.Time, passwordKDFMaxTime)
	}
	if p.Parallelism < 1 || p.Parallelism > passwordKDFMaxParallelism {
		return errors.Errorf("argon2id parallelism %d out of range [1, %d]",
			p.Parallelism, passwordKDFMaxParallelism)
	}
	if p.MemoryKiB < 8*uint32(p.Parallelism) || p.MemoryKiB > passwordKDFMaxMemoryKiB {
		return errors.Errorf("argon2id memory %d KiB out of range [%d, %d]",
			p.MemoryKiB, 8*uint32(p.Parallelism), passwordKDFMaxMemoryKiB)
	}

	return nil
}

// validateForEncrypt applies the hard bounds plus the encryption floor
// (t >= 2, m >= 19 MiB). It returns an error when the parameters are too weak
// or exceed the bounds that decryption accepts.
func (p PasswordKDFParams) validateForEncrypt() error {
	if err := p.validateForDecrypt(); err != nil {
		return errors.WithStack(err)
	}
	if p.Time < passwordKDFMinTime || p.MemoryKiB < passwordKDFMinMemoryKiB {
		return errors.Errorf("argon2id parameters below the minimum (time >= %d, memory >= %d KiB)",
			passwordKDFMinTime, passwordKDFMinMemoryKiB)
	}

	return nil
}

// passwordEncryptorOption holds NewPasswordEncryptor configuration.
type passwordEncryptorOption struct {
	params PasswordKDFParams
	// unchecked skips the encryption floor; it is only set by package tests.
	unchecked bool
}

// PasswordEncryptorOption configures NewPasswordEncryptor.
type PasswordEncryptorOption func(*passwordEncryptorOption) error

// WithPasswordKDFParams overrides the Argon2id parameters. It returns an option
// that fails when params are below the floor (t >= 2, m >= 19 MiB) or above the
// decryption bounds (t <= 10, m <= 256 MiB, 1 <= p <= 16).
func WithPasswordKDFParams(params PasswordKDFParams) PasswordEncryptorOption {
	return func(o *passwordEncryptorOption) error {
		if err := params.validateForEncrypt(); err != nil {
			return errors.WithStack(err)
		}

		o.params = params
		return nil
	}
}

// PasswordEncryptor encrypts many messages under one password with a single
// Argon2id derivation. NewPasswordEncryptor draws a fresh random salt and
// derives the key once; every Encrypt call uses a fresh random nonce and emits a
// self-describing ciphertext (header, salt and KDF parameters included) that
// DecryptByPassword can open on its own. This lets a directory run pay the KDF
// cost once while keeping each output file independent. It is safe for
// concurrent use.
type PasswordEncryptor struct {
	header []byte
	key    []byte
	count  atomic.Uint64
}

// NewPasswordEncryptor derives an AES-256-GCM key from password with Argon2id
// under a fresh 16-byte random salt. The password must not be empty. It returns
// the encryptor, or an error for an empty password, invalid option or
// randomness failure.
func NewPasswordEncryptor(password []byte, opts ...PasswordEncryptorOption) (*PasswordEncryptor, error) {
	if len(password) == 0 {
		return nil, errors.New("password must not be empty")
	}

	opt := passwordEncryptorOption{params: DefaultPasswordKDFParams()}
	for _, f := range opts {
		if err := f(&opt); err != nil {
			return nil, errors.Wrap(err, "apply password encryptor option")
		}
	}
	validate := opt.params.validateForEncrypt
	if opt.unchecked {
		validate = opt.params.validateForDecrypt
	}
	if err := validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	salt, err := Salt(passwordSaltLen)
	if err != nil {
		return nil, errors.Wrap(err, "generate password salt")
	}

	header := make([]byte, 0, passwordHeaderLen)
	header = append(header, passwordFormatMagic...)
	header = append(header, passwordFormatVersion1, passwordKDFArgon2id)
	header = binary.BigEndian.AppendUint32(header, opt.params.Time)
	header = binary.BigEndian.AppendUint32(header, opt.params.MemoryKiB)
	header = append(header, opt.params.Parallelism)
	header = append(header, salt...)

	key := argon2.IDKey(password, salt, opt.params.Time, opt.params.MemoryKiB,
		opt.params.Parallelism, passwordKeyLen)
	return &PasswordEncryptor{header: header, key: key}, nil
}

// Encrypt seals plaintext (which may be empty) with a fresh random nonce and
// returns header || nonce || ciphertext || tag. It returns an error when the
// plaintext exceeds the AES-GCM limit, randomness fails, or the encryptor has
// produced 2^32 messages and must be replaced.
func (e *PasswordEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if e == nil || len(e.key) != passwordKeyLen {
		return nil, errors.New("password encryptor is not initialized")
	}
	if uint64(len(plaintext)) > maxGCMPlaintextLen {
		return nil, errors.Errorf("plaintext too large for AES-GCM: %d bytes", len(plaintext))
	}
	if e.count.Add(1) > passwordEncryptorMaxMessages {
		return nil, errors.New("password encryptor message limit reached; create a new encryptor")
	}

	aead, err := newPasswordAEAD(e.key)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	nonce, err := Salt(AesGcmIvLen)
	if err != nil {
		return nil, errors.Wrap(err, "generate nonce")
	}

	out := make([]byte, 0, passwordOverhead+len(plaintext))
	out = append(out, e.header...)
	out = append(out, nonce...)
	return aead.Seal(out, nonce, plaintext, e.header), nil
}

// EncryptByPassword encrypts plaintext under password with the versioned
// Argon2id + AES-256-GCM format, using default KDF parameters and a fresh salt
// and nonce. It returns the self-describing ciphertext or an error.
func EncryptByPassword(password, plaintext []byte, opts ...PasswordEncryptorOption) ([]byte, error) {
	enc, err := NewPasswordEncryptor(password, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return ciphertext, nil
}

// IsPasswordEncrypted reports whether data starts with the password-format
// magic. It does not authenticate anything.
func IsPasswordEncrypted(data []byte) bool {
	return bytes.HasPrefix(data, []byte(passwordFormatMagic))
}

// DecryptByPassword opens a ciphertext produced by EncryptByPassword or
// PasswordEncryptor.Encrypt. Header fields are validated and the Argon2id
// parameters are bounded (t <= 10, m <= 256 MiB, 1 <= p <= 16) before any key
// derivation. It returns the plaintext, or an error for malformed input,
// unsupported versions, hostile parameters, a wrong password or tampering.
func DecryptByPassword(password, ciphertext []byte) ([]byte, error) {
	if len(password) == 0 {
		return nil, errors.New("password must not be empty")
	}

	params, err := parsePasswordHeader(ciphertext)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	header := ciphertext[:passwordHeaderLen]
	salt := header[passwordHeaderLen-passwordSaltLen:]
	nonce := ciphertext[passwordHeaderLen : passwordHeaderLen+AesGcmIvLen]
	sealed := ciphertext[passwordHeaderLen+AesGcmIvLen:]

	key := argon2.IDKey(password, salt, params.Time, params.MemoryKiB, params.Parallelism, passwordKeyLen)
	defer clear(key)

	aead, err := newPasswordAEAD(key)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	plaintext, err := aead.Open(nil, nonce, sealed, header)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt password-encrypted data: wrong password or tampered data")
	}

	return plaintext, nil
}

// parsePasswordHeader validates the untrusted header of ciphertext and returns
// its KDF parameters. It returns an error for short input, bad magic, unknown
// version or KDF, or out-of-range parameters.
func parsePasswordHeader(ciphertext []byte) (PasswordKDFParams, error) {
	if !IsPasswordEncrypted(ciphertext) {
		return PasswordKDFParams{}, errors.New("not password-encrypted data: bad magic")
	}
	if len(ciphertext) < passwordOverhead {
		return PasswordKDFParams{}, errors.New("password-encrypted data is truncated")
	}

	off := len(passwordFormatMagic)
	if v := ciphertext[off]; v != passwordFormatVersion1 {
		return PasswordKDFParams{}, errors.Errorf("unsupported password format version %d", v)
	}
	if kdf := ciphertext[off+1]; kdf != passwordKDFArgon2id {
		return PasswordKDFParams{}, errors.Errorf("unsupported password KDF id %d", kdf)
	}

	params := PasswordKDFParams{
		Time:        binary.BigEndian.Uint32(ciphertext[off+2 : off+6]),
		MemoryKiB:   binary.BigEndian.Uint32(ciphertext[off+6 : off+10]),
		Parallelism: ciphertext[off+10],
	}
	if err := params.validateForDecrypt(); err != nil {
		return PasswordKDFParams{}, errors.Wrap(err, "reject password KDF parameters")
	}

	return params, nil
}

// newPasswordAEAD returns AES-256-GCM for a 32-byte derived key, or an error
// when the cipher cannot be constructed.
func newPasswordAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "new aes cipher")
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "new gcm")
	}

	return aead, nil
}
