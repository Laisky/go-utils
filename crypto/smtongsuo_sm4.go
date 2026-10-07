package crypto

import (
	"bytes"
	"context"
	"crypto/cipher"
	cryptohmac "crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/emmansun/gmsm/sm4"

	glog "github.com/Laisky/go-utils/v6/log"
)

// SM4-CBC formats implemented in this file.
//
// All SM4 operations run in-process with github.com/emmansun/gmsm/sm4 and
// crypto/cipher CBC mode with PKCS#7 padding. The output is byte-for-byte
// identical to `tongsuo enc -sm4-cbc -K <key> -iv <iv>`, but no key material is
// ever passed to a child process. The methods stay on *Tongsuo for API
// compatibility only; they never invoke the tongsuo binary.
//
// Versioned envelope (EncryptBySm4Cbc / DecryptBySm4Cbc):
//
//	magic("GUS4") || version(0x01) || iv(16) || ciphertext(16*n, n>=1) || tag(32)
//
// encKey and macKey are derived from the caller key with HKDF-SHA256 using
// distinct info labels; ciphertext = SM4-CBC-PKCS7(encKey, iv, plaintext) and
// tag = HMAC-SHA256(macKey, magic || version || iv || ciphertext).
//
// Legacy envelope (DecryptBySm4CbcLegacy only):
//
//	iv(16) || ciphertext(16*n, n>=1) || HMAC-SHA256(key, ciphertext)(32)
//
// The legacy MAC does not authenticate the IV, so an attacker who can modify the
// stored bytes can flip bits of the first plaintext block undetected.
const (
	// sm4KeySize is the SM4 key size in bytes.
	sm4KeySize = 16
	// sm4IVSize is the SM4-CBC IV size in bytes, equal to the SM4 block size.
	sm4IVSize = sm4.BlockSize
	// sm4MACSize is the HMAC-SHA256 tag size in bytes.
	sm4MACSize = sha256.Size
	// sm4EnvelopeMagic identifies the versioned SM4-CBC envelope.
	sm4EnvelopeMagic = "GUS4"
	// sm4EnvelopeVersion1 identifies SM4-CBC-PKCS7 + HMAC-SHA256 encrypt-then-MAC
	// with HKDF-SHA256 derived encryption and MAC keys.
	sm4EnvelopeVersion1 byte = 0x01
	// sm4EnvelopeHeaderSize is the size of magic || version.
	sm4EnvelopeHeaderSize = len(sm4EnvelopeMagic) + 1
	// sm4EnvelopeMinSize is the size of a v1 envelope holding one ciphertext block.
	sm4EnvelopeMinSize = sm4EnvelopeHeaderSize + sm4IVSize + sm4.BlockSize + sm4MACSize
	// sm4LegacyEnvelopeMinSize is the size of a legacy envelope holding one ciphertext block.
	sm4LegacyEnvelopeMinSize = sm4IVSize + sm4.BlockSize + sm4MACSize

	// sm4EnvelopeEncKeyInfo is the HKDF info label of the v1 envelope encryption key.
	sm4EnvelopeEncKeyInfo = "github.com/Laisky/go-utils/crypto sm4-cbc-hmac-sha256 envelope v1 encryption key"
	// sm4EnvelopeMACKeyInfo is the HKDF info label of the v1 envelope MAC key.
	sm4EnvelopeMACKeyInfo = "github.com/Laisky/go-utils/crypto sm4-cbc-hmac-sha256 envelope v1 mac key"
	// sm4BasicMACKeyInfo is the HKDF info label of the basic API MAC key.
	sm4BasicMACKeyInfo = "github.com/Laisky/go-utils/crypto sm4-cbc basic iv-and-ciphertext v1 mac key"
)

var (
	// ErrSm4CbcHMACMismatch is returned when an SM4-CBC MAC does not verify.
	// No plaintext is ever returned together with this error.
	ErrSm4CbcHMACMismatch = errors.New("hmac not match")
	// ErrSm4CbcMalformedCiphertext is returned when an SM4-CBC envelope or
	// ciphertext has an invalid length, magic, or version. It is detected before
	// any cryptographic operation.
	ErrSm4CbcMalformedCiphertext = errors.New("malformed sm4-cbc ciphertext")
	// ErrSm4CbcBadDecrypt is returned when SM4-CBC decryption yields invalid
	// PKCS#7 padding, which usually means a wrong key or corrupted data.
	ErrSm4CbcBadDecrypt = errors.New("bad decrypt")
)

// EncryptBySm4CbcBaisc encrypts plaintext with SM4-CBC and PKCS#7 padding in-process.
//
// The ciphertext is standard SM4-CBC under the raw key and IV, byte-for-byte
// identical to `tongsuo enc -sm4-cbc -K <key> -iv <iv>`, so other SM4
// implementations can decrypt it. The tongsuo binary is not invoked and the key
// never appears in any process argv.
//
// The returned hmac is HMAC-SHA256 over iv || ciphertext under a MAC key derived
// from key with HKDF-SHA256 and a dedicated info label, so both the IV and the
// ciphertext are authenticated. Verify it with DecryptBySm4CbcBaisc. MACs produced
// by older releases (HMAC-SHA256(key, ciphertext)) can only be verified with
// DecryptBySm4CbcBaiscLegacy.
//
// Never reuse an IV with the same key; prefer EncryptBySm4Cbc, which generates a
// random IV and emits a self-describing authenticated envelope.
//
// # Args
//   - ctx: context checked for cancellation before encrypting.
//   - key: SM4 key, must be 16 bytes.
//   - plaintext: data to be encrypted, may be empty (yields one padding block).
//   - iv: SM4-CBC IV, must be 16 bytes and unique per key.
//
// # Returns
//   - ciphertext: SM4-CBC ciphertext, a nonzero multiple of 16 bytes.
//   - hmac: 32-byte HMAC-SHA256 tag over iv || ciphertext.
//   - err: non-nil when the context is done or an argument is invalid.
func (t *Tongsuo) EncryptBySm4CbcBaisc(ctx context.Context,
	key, plaintext, iv []byte) (ciphertext, hmac []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, nil, errors.Wrap(err, "context done")
	}
	if err = validateSm4KeyAndIV(key, iv); err != nil {
		return nil, nil, errors.Wrap(err, "validate sm4 arguments")
	}

	if ciphertext, err = sm4CbcEncrypt(key, iv, plaintext); err != nil {
		return nil, nil, errors.Wrap(err, "encrypt")
	}

	macKey, err := deriveSm4Key(key, sm4BasicMACKeyInfo, sm4MACSize)
	if err != nil {
		return nil, nil, errors.Wrap(err, "derive mac key")
	}

	return ciphertext, sm4HMAC(macKey, iv, ciphertext), nil
}

// DecryptBySm4CbcBaisc decrypts standard SM4-CBC ciphertext with PKCS#7 padding in-process.
//
// When hmac is nonempty it must be the 32-byte tag returned by
// EncryptBySm4CbcBaisc; it is verified in constant time over iv || ciphertext
// BEFORE any decryption, so a modified IV, ciphertext, or tag fails with
// ErrSm4CbcHMACMismatch and no plaintext. Tags produced by older releases, which
// did not cover the IV, are rejected here; use DecryptBySm4CbcBaiscLegacy to
// migrate such data.
//
// WARNING: an empty hmac means NO integrity check at all. The ciphertext and IV
// are then fully malleable and the padding check may act as a padding oracle.
// Only pass an empty hmac for interoperability with data authenticated by other
// means.
//
// The tongsuo binary is not invoked and the key never appears in any process argv.
//
// # Args
//   - ctx: context checked for cancellation before decrypting.
//   - key: SM4 key, must be 16 bytes.
//   - ciphertext: SM4-CBC ciphertext, must be a nonzero multiple of 16 bytes.
//   - iv: SM4-CBC IV, must be 16 bytes.
//   - hmac: empty to skip integrity checks, otherwise the 32-byte tag over iv || ciphertext.
//
// # Returns
//   - plaintext: decrypted data, only returned when every check succeeded.
//   - err: non-nil on invalid arguments, MAC mismatch, or invalid padding.
func (t *Tongsuo) DecryptBySm4CbcBaisc(ctx context.Context,
	key, ciphertext, iv, hmac []byte) (plaintext []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if err = validateSm4DecryptArgs(key, ciphertext, iv, hmac); err != nil {
		return nil, errors.Wrap(err, "validate sm4 arguments")
	}

	if len(hmac) != 0 {
		macKey, err := deriveSm4Key(key, sm4BasicMACKeyInfo, sm4MACSize)
		if err != nil {
			return nil, errors.Wrap(err, "derive mac key")
		}
		if !cryptohmac.Equal(hmac, sm4HMAC(macKey, iv, ciphertext)) {
			return nil, errors.WithStack(ErrSm4CbcHMACMismatch)
		}
	}

	if plaintext, err = sm4CbcDecrypt(key, iv, ciphertext); err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	return plaintext, nil
}

// DecryptBySm4CbcBaiscLegacy decrypts SM4-CBC ciphertext whose MAC was produced
// by releases before the IV-authenticating format, i.e. HMAC-SHA256(key, ciphertext).
//
// SECURITY: the legacy MAC does NOT authenticate the IV. An attacker who can
// modify the stored IV can flip arbitrary bits of the first 16 plaintext bytes
// and this function still succeeds. Use it only to migrate existing data:
// decrypt with this function, then re-encrypt with EncryptBySm4CbcBaisc (or
// preferably EncryptBySm4Cbc). Track the format of stored data out of band and
// never fall back to this function automatically when the current format fails.
//
// The MAC is verified in constant time before decryption. The tongsuo binary is
// not invoked and the key never appears in any process argv.
//
// # Args
//   - ctx: context checked for cancellation before decrypting.
//   - key: SM4 key, must be 16 bytes.
//   - ciphertext: SM4-CBC ciphertext, must be a nonzero multiple of 16 bytes.
//   - iv: SM4-CBC IV, must be 16 bytes; it is NOT authenticated.
//   - hmac: legacy 32-byte HMAC-SHA256(key, ciphertext) tag, required.
//
// # Returns
//   - plaintext: decrypted data, only returned when every check succeeded.
//   - err: non-nil on invalid arguments, MAC mismatch, or invalid padding.
func (t *Tongsuo) DecryptBySm4CbcBaiscLegacy(ctx context.Context,
	key, ciphertext, iv, hmac []byte) (plaintext []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if len(hmac) == 0 {
		return nil, errors.Errorf("hmac should be 32 bytes for legacy verification")
	}
	if err = validateSm4DecryptArgs(key, ciphertext, iv, hmac); err != nil {
		return nil, errors.Wrap(err, "validate sm4 arguments")
	}

	expected, err := HMACSha256(key, bytes.NewReader(ciphertext))
	if err != nil {
		return nil, errors.Wrap(err, "calculate legacy hmac")
	}
	if !cryptohmac.Equal(hmac, expected) {
		return nil, errors.WithStack(ErrSm4CbcHMACMismatch)
	}

	glog.Shared.Debug("decrypt legacy sm4-cbc data whose iv is not authenticated",
		zap.Int("ciphertext_len", len(ciphertext)))
	if plaintext, err = sm4CbcDecrypt(key, iv, ciphertext); err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	return plaintext, nil
}

// EncryptBySm4Cbc encrypts plaintext into a versioned, fully authenticated SM4-CBC envelope.
//
// The output format is magic("GUS4") || version(0x01) || iv(16) || ciphertext ||
// tag(32). Independent SM4 encryption and HMAC-SHA256 keys are derived from key
// with HKDF-SHA256 and distinct info labels, a fresh random IV is generated, and
// the tag authenticates magic, version, IV, and ciphertext. Decrypt the result
// with DecryptBySm4Cbc only.
//
// The tongsuo binary is not invoked and the key never appears in any process argv.
//
// # Args
//   - ctx: context checked for cancellation before encrypting.
//   - key: SM4 key, must be 16 bytes.
//   - plaintext: data to be encrypted, may be empty.
//
// # Returns
//   - combinedCipher: the v1 envelope, at least 69 bytes.
//   - err: non-nil when the context is done, the key is invalid, or randomness fails.
func (t *Tongsuo) EncryptBySm4Cbc(ctx context.Context, key, plaintext []byte) (
	combinedCipher []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if err = validateSm4Key(key); err != nil {
		return nil, errors.Wrap(err, "validate sm4 key")
	}

	encKey, macKey, err := deriveSm4EnvelopeKeys(key)
	if err != nil {
		return nil, errors.Wrap(err, "derive envelope keys")
	}

	iv, err := Salt(sm4IVSize)
	if err != nil {
		return nil, errors.Wrap(err, "generate iv")
	}

	ciphertext, err := sm4CbcEncrypt(encKey, iv, plaintext)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt")
	}

	combinedCipher = make([]byte, 0, sm4EnvelopeHeaderSize+sm4IVSize+len(ciphertext)+sm4MACSize)
	combinedCipher = append(combinedCipher, sm4EnvelopeMagic...)
	combinedCipher = append(combinedCipher, sm4EnvelopeVersion1)
	combinedCipher = append(combinedCipher, iv...)
	combinedCipher = append(combinedCipher, ciphertext...)
	combinedCipher = append(combinedCipher, sm4HMAC(macKey, combinedCipher)...)

	return combinedCipher, nil
}

// DecryptBySm4Cbc authenticates and decrypts a v1 envelope produced by EncryptBySm4Cbc.
//
// Length, magic, and version are validated before any cryptographic operation,
// and the tag over magic || version || iv || ciphertext is verified in constant
// time BEFORE decryption. Any modification of the header, IV, ciphertext, or tag
// fails without returning plaintext. Only the v1 envelope is accepted: data
// produced by older releases (iv || ciphertext || HMAC(key, ciphertext)) is
// rejected and must be migrated explicitly with DecryptBySm4CbcLegacy.
//
// The tongsuo binary is not invoked and the key never appears in any process argv.
//
// # Args
//   - ctx: context checked for cancellation before decrypting.
//   - key: SM4 key, must be 16 bytes.
//   - combinedCipher: the v1 envelope returned by EncryptBySm4Cbc.
//
// # Returns
//   - plaintext: decrypted data, only returned when authentication succeeded.
//   - err: ErrSm4CbcMalformedCiphertext for malformed input, ErrSm4CbcHMACMismatch
//     for authentication failures, or another error for invalid arguments.
func (t *Tongsuo) DecryptBySm4Cbc(ctx context.Context, key, combinedCipher []byte) (
	plaintext []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if err = validateSm4Key(key); err != nil {
		return nil, errors.Wrap(err, "validate sm4 key")
	}
	if err = validateSm4Envelope(combinedCipher); err != nil {
		return nil, errors.Wrap(err, "parse sm4 envelope")
	}

	authenticated := combinedCipher[:len(combinedCipher)-sm4MACSize]
	tag := combinedCipher[len(combinedCipher)-sm4MACSize:]
	iv := authenticated[sm4EnvelopeHeaderSize : sm4EnvelopeHeaderSize+sm4IVSize]
	ciphertext := authenticated[sm4EnvelopeHeaderSize+sm4IVSize:]

	encKey, macKey, err := deriveSm4EnvelopeKeys(key)
	if err != nil {
		return nil, errors.Wrap(err, "derive envelope keys")
	}
	if !cryptohmac.Equal(tag, sm4HMAC(macKey, authenticated)) {
		return nil, errors.WithStack(ErrSm4CbcHMACMismatch)
	}

	if plaintext, err = sm4CbcDecrypt(encKey, iv, ciphertext); err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	return plaintext, nil
}

// DecryptBySm4CbcLegacy decrypts the combined format produced by EncryptBySm4Cbc
// in releases before the versioned envelope: iv(16) || ciphertext || HMAC-SHA256(key, ciphertext)(32).
//
// SECURITY: the legacy MAC does NOT authenticate the IV, so an attacker who can
// modify the stored bytes can flip arbitrary bits of the first 16 plaintext bytes
// and this function still succeeds. Use it only for migration: decrypt legacy
// data with this function and immediately re-encrypt it with EncryptBySm4Cbc.
// Track the format of stored data out of band and never call this function as an
// automatic fallback after DecryptBySm4Cbc fails on attacker-reachable input.
//
// The function rejects v1 envelopes. The tongsuo binary is not invoked and the
// key never appears in any process argv.
//
// # Args
//   - ctx: context checked for cancellation before decrypting.
//   - key: SM4 key, must be 16 bytes.
//   - combinedCipher: legacy iv || ciphertext || hmac bytes.
//
// # Returns
//   - plaintext: decrypted data, only returned when the legacy MAC verified.
//   - err: non-nil on malformed input, MAC mismatch, or invalid padding.
func (t *Tongsuo) DecryptBySm4CbcLegacy(ctx context.Context, key, combinedCipher []byte) (
	plaintext []byte, err error) {
	if len(combinedCipher) < sm4LegacyEnvelopeMinSize ||
		(len(combinedCipher)-sm4IVSize-sm4MACSize)%sm4.BlockSize != 0 {
		if validateSm4Envelope(combinedCipher) == nil {
			return nil, errors.Wrap(ErrSm4CbcMalformedCiphertext,
				"input is a v1 sm4 envelope, decrypt it with DecryptBySm4Cbc")
		}

		return nil, errors.Wrapf(ErrSm4CbcMalformedCiphertext,
			"legacy sm4 envelope length %d is invalid", len(combinedCipher))
	}

	iv := combinedCipher[:sm4IVSize]
	ciphertext := combinedCipher[sm4IVSize : len(combinedCipher)-sm4MACSize]
	hmac := combinedCipher[len(combinedCipher)-sm4MACSize:]

	return t.DecryptBySm4CbcBaiscLegacy(ctx, key, ciphertext, iv, hmac)
}

// validateSm4Key checks that key has the SM4 key size. It returns a descriptive
// error when the size is wrong, or nil.
func validateSm4Key(key []byte) error {
	if len(key) != sm4KeySize {
		return errors.Errorf("key should be %d bytes", sm4KeySize)
	}

	return nil
}

// validateSm4KeyAndIV checks that key and iv have the SM4 key and block sizes.
// It returns a descriptive error for the first invalid argument, or nil.
func validateSm4KeyAndIV(key, iv []byte) error {
	if err := validateSm4Key(key); err != nil {
		return err
	}
	if len(iv) != sm4IVSize {
		return errors.Errorf("iv should be 16 bytes")
	}

	return nil
}

// validateSm4DecryptArgs checks the key, IV, MAC, and ciphertext lengths of a
// basic SM4-CBC decryption before any cryptographic operation. It returns a
// descriptive error for the first invalid argument, or nil.
func validateSm4DecryptArgs(key, ciphertext, iv, hmac []byte) error {
	if err := validateSm4KeyAndIV(key, iv); err != nil {
		return err
	}
	if len(hmac) != 0 && len(hmac) != sm4MACSize {
		return errors.Errorf("hmac should be 0 or 32 bytes")
	}
	if len(ciphertext) == 0 || len(ciphertext)%sm4.BlockSize != 0 {
		return errors.Wrapf(ErrSm4CbcMalformedCiphertext,
			"ciphertext length %d is not a nonzero multiple of %d", len(ciphertext), sm4.BlockSize)
	}

	return nil
}

// validateSm4Envelope checks the length, magic, and version of a v1 envelope
// without performing any cryptographic operation. It returns an error wrapping
// ErrSm4CbcMalformedCiphertext when the envelope is malformed, or nil.
func validateSm4Envelope(envelope []byte) error {
	if len(envelope) < sm4EnvelopeMinSize ||
		(len(envelope)-sm4EnvelopeHeaderSize-sm4IVSize-sm4MACSize)%sm4.BlockSize != 0 {
		return errors.Wrapf(ErrSm4CbcMalformedCiphertext, "envelope length %d is invalid", len(envelope))
	}
	if subtle.ConstantTimeCompare(envelope[:len(sm4EnvelopeMagic)], []byte(sm4EnvelopeMagic)) != 1 {
		return errors.Wrap(ErrSm4CbcMalformedCiphertext, "envelope magic mismatch")
	}
	if version := envelope[len(sm4EnvelopeMagic)]; version != sm4EnvelopeVersion1 {
		return errors.Wrapf(ErrSm4CbcMalformedCiphertext, "unsupported envelope version %d", version)
	}

	return nil
}

// deriveSm4Key derives a length-byte subkey from key with HKDF-SHA256 using the
// given info label and an empty salt. It returns the subkey or an error.
func deriveSm4Key(key []byte, info string, length int) ([]byte, error) {
	subkey := make([]byte, length)
	if err := HKDFWithSHA256(key, nil, []byte(info), [][]byte{subkey}); err != nil {
		return nil, errors.Wrap(err, "hkdf")
	}

	return subkey, nil
}

// deriveSm4EnvelopeKeys derives the independent v1 envelope SM4 encryption key
// and HMAC-SHA256 key from key. It returns both keys or an error.
func deriveSm4EnvelopeKeys(key []byte) (encKey, macKey []byte, err error) {
	if encKey, err = deriveSm4Key(key, sm4EnvelopeEncKeyInfo, sm4KeySize); err != nil {
		return nil, nil, errors.Wrap(err, "derive encryption key")
	}
	if macKey, err = deriveSm4Key(key, sm4EnvelopeMACKeyInfo, sm4MACSize); err != nil {
		return nil, nil, errors.Wrap(err, "derive mac key")
	}

	return encKey, macKey, nil
}

// sm4HMAC returns HMAC-SHA256 under macKey over the concatenation of parts.
// Callers must only concatenate fixed-length parts or a single variable-length
// trailing part so that the MAC input is unambiguous.
func sm4HMAC(macKey []byte, parts ...[]byte) []byte {
	mac := cryptohmac.New(sha256.New, macKey)
	for _, part := range parts {
		mac.Write(part)
	}

	return mac.Sum(nil)
}

// sm4CbcEncrypt encrypts plaintext with SM4-CBC and PKCS#7 padding, matching
// `tongsuo enc -sm4-cbc -K -iv`. It returns a nonzero multiple of 16 bytes, or
// an error when key or iv has an invalid size.
func sm4CbcEncrypt(key, iv, plaintext []byte) ([]byte, error) {
	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "new sm4 cipher")
	}
	if len(iv) != block.BlockSize() {
		return nil, errors.Errorf("iv should be %d bytes", block.BlockSize())
	}

	padLen := sm4.BlockSize - len(plaintext)%sm4.BlockSize
	buf := make([]byte, len(plaintext)+padLen)
	copy(buf, plaintext)
	for i := len(plaintext); i < len(buf); i++ {
		buf[i] = byte(padLen)
	}

	cipher.NewCBCEncrypter(block, iv).CryptBlocks(buf, buf)

	return buf, nil
}

// sm4CbcDecrypt decrypts SM4-CBC ciphertext and strictly removes PKCS#7 padding
// in constant time. It returns the plaintext, or an error wrapping
// ErrSm4CbcBadDecrypt when the padding is invalid; on error the decrypted
// buffer is wiped and never returned.
func sm4CbcDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(ciphertext)%sm4.BlockSize != 0 {
		return nil, errors.Wrapf(ErrSm4CbcMalformedCiphertext,
			"ciphertext length %d is not a nonzero multiple of %d", len(ciphertext), sm4.BlockSize)
	}

	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "new sm4 cipher")
	}
	if len(iv) != block.BlockSize() {
		return nil, errors.Errorf("iv should be %d bytes", block.BlockSize())
	}

	buf := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(buf, ciphertext)

	padLen, ok := pkcs7PaddingLen(buf, sm4.BlockSize)
	if !ok {
		clear(buf)
		return nil, errors.WithStack(ErrSm4CbcBadDecrypt)
	}

	return buf[:len(buf)-padLen], nil
}

// pkcs7PaddingLen validates PKCS#7 padding on data, whose length must be a
// nonzero multiple of blockSize, without data-dependent branches or memory
// access. It returns the padding length and true when the padding is valid,
// otherwise zero and false.
func pkcs7PaddingLen(data []byte, blockSize int) (int, bool) {
	n := len(data)
	if n == 0 || n%blockSize != 0 {
		return 0, false
	}

	padByte := data[n-1]
	padLen := int(padByte)
	good := subtle.ConstantTimeLessOrEq(1, padLen) & subtle.ConstantTimeLessOrEq(padLen, blockSize)
	for i := 1; i <= blockSize; i++ {
		inPadding := subtle.ConstantTimeLessOrEq(i, padLen)
		matches := subtle.ConstantTimeByteEq(data[n-i], padByte)
		good &= subtle.ConstantTimeSelect(inPadding, matches, 1)
	}

	if good != 1 {
		return 0, false
	}

	return padLen, true
}
