package crypto

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// passwordTestPassword is a synthetic, human-readable 16-byte test password.
const passwordTestPassword = "testpassword1234"

// fastPasswordKDF returns a package-test option with tiny Argon2id parameters
// (t=1, m=64 KiB, p=1) that bypasses the encryption floor to keep tests fast.
func fastPasswordKDF() PasswordEncryptorOption {
	return func(o *passwordEncryptorOption) error {
		o.params = PasswordKDFParams{Time: 1, MemoryKiB: 64, Parallelism: 1}
		o.unchecked = true
		return nil
	}
}

// passwordSalt returns the salt bytes embedded in a password-format ciphertext.
func passwordSalt(ct []byte) []byte {
	return ct[passwordHeaderLen-passwordSaltLen : passwordHeaderLen]
}

// passwordNonce returns the nonce bytes embedded in a password-format ciphertext.
func passwordNonce(ct []byte) []byte {
	return ct[passwordHeaderLen : passwordHeaderLen+AesGcmIvLen]
}

// passwordTamperMask returns the bit mask used to tamper with byte i of a
// password-format ciphertext. The high-order Argon2id memory bytes use a bit
// that yields an out-of-range (rejected) value instead of a valid but expensive
// one, keeping the exhaustive tamper test fast; every other byte flips bit 0.
func passwordTamperMask(i int) byte {
	memory := len(passwordFormatMagic) + 6
	if i == memory || i == memory+1 {
		return 0x10
	}
	return 0x01
}

// TestPasswordSecurity_PasswordIsNotTheKey checks that password mode never uses
// the password bytes directly as the AES key. Regression for issue #54.
func TestPasswordSecurity_PasswordIsNotTheKey(t *testing.T) {
	t.Parallel()

	msg := []byte("secret config")
	ct, err := EncryptByPassword([]byte(passwordTestPassword), msg, fastPasswordKDF())
	require.NoError(t, err)
	require.True(t, IsPasswordEncrypted(ct))

	_, err = AEADDecrypt([]byte(passwordTestPassword), ct, nil)
	require.Error(t, err)
	_, err = AEADDecrypt([]byte(passwordTestPassword), ct[passwordHeaderLen:], nil)
	require.Error(t, err)
	_, err = AEADDecrypt([]byte(passwordTestPassword), ct[passwordHeaderLen:], ct[:passwordHeaderLen])
	require.Error(t, err)

	got, err := DecryptByPassword([]byte(passwordTestPassword), ct)
	require.NoError(t, err)
	require.Equal(t, msg, got)
}

// TestPasswordSecurity_FreshSaltAndNonce checks that reusing a password yields
// independent, decryptable outputs. Regression for issue #54.
func TestPasswordSecurity_FreshSaltAndNonce(t *testing.T) {
	t.Parallel()

	msg := []byte("same message")
	a, err := EncryptByPassword([]byte(passwordTestPassword), msg, fastPasswordKDF())
	require.NoError(t, err)
	b, err := EncryptByPassword([]byte(passwordTestPassword), msg, fastPasswordKDF())
	require.NoError(t, err)
	require.NotEqual(t, passwordSalt(a), passwordSalt(b))
	require.NotEqual(t, passwordNonce(a), passwordNonce(b))

	for _, ct := range [][]byte{a, b} {
		got, err := DecryptByPassword([]byte(passwordTestPassword), ct)
		require.NoError(t, err)
		require.Equal(t, msg, got)
	}

	// One encryptor derives once: shared salt, fresh nonce per message.
	enc, err := NewPasswordEncryptor([]byte(passwordTestPassword), fastPasswordKDF())
	require.NoError(t, err)
	c1, err := enc.Encrypt(msg)
	require.NoError(t, err)
	c2, err := enc.Encrypt(nil)
	require.NoError(t, err)
	require.Equal(t, passwordSalt(c1), passwordSalt(c2))
	require.NotEqual(t, passwordNonce(c1), passwordNonce(c2))
	got, err := DecryptByPassword([]byte(passwordTestPassword), c1)
	require.NoError(t, err)
	require.Equal(t, msg, got)
	got, err = DecryptByPassword([]byte(passwordTestPassword), c2)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestPasswordSecurity_WrongPasswordAndTampering checks that a wrong password
// and any single-byte change of header, nonce, ciphertext or tag fail.
// Regression for issue #54.
func TestPasswordSecurity_WrongPasswordAndTampering(t *testing.T) {
	t.Parallel()

	ct, err := EncryptByPassword([]byte(passwordTestPassword), []byte("payload"), fastPasswordKDF())
	require.NoError(t, err)

	_, err = DecryptByPassword([]byte("testpassword1235"), ct)
	require.Error(t, err)
	_, err = DecryptByPassword(nil, ct)
	require.Error(t, err)

	for i := range ct {
		tampered := append([]byte(nil), ct...)
		tampered[i] ^= passwordTamperMask(i)
		_, err := DecryptByPassword([]byte(passwordTestPassword), tampered)
		require.Error(t, err, "tampered byte %d accepted", i)
	}

	for cut := range len(ct) {
		_, err := DecryptByPassword([]byte(passwordTestPassword), ct[:cut])
		require.Error(t, err, "truncated to %d accepted", cut)
	}
	_, err = DecryptByPassword([]byte(passwordTestPassword), append(append([]byte(nil), ct...), 0))
	require.Error(t, err, "trailing byte accepted")
}

// TestPasswordSecurity_HostileParamsRejectedBeforeKDF checks that untrusted KDF
// parameters are bounded before any expensive Argon2id work. Regression for
// issue #54.
func TestPasswordSecurity_HostileParamsRejectedBeforeKDF(t *testing.T) {
	t.Parallel()

	ct, err := EncryptByPassword([]byte(passwordTestPassword), []byte("payload"), fastPasswordKDF())
	require.NoError(t, err)

	off := len(passwordFormatMagic)
	cases := map[string]func(h []byte){
		"version":         func(h []byte) { h[off] = 2 },
		"kdf id":          func(h []byte) { h[off+1] = 9 },
		"time zero":       func(h []byte) { binary.BigEndian.PutUint32(h[off+2:], 0) },
		"time huge":       func(h []byte) { binary.BigEndian.PutUint32(h[off+2:], math.MaxUint32) },
		"time 11":         func(h []byte) { binary.BigEndian.PutUint32(h[off+2:], passwordKDFMaxTime+1) },
		"memory huge":     func(h []byte) { binary.BigEndian.PutUint32(h[off+6:], math.MaxUint32) },
		"memory 257 MiB":  func(h []byte) { binary.BigEndian.PutUint32(h[off+6:], passwordKDFMaxMemoryKiB+1) },
		"memory below 8p": func(h []byte) { binary.BigEndian.PutUint32(h[off+6:], 7) },
		"lanes zero":      func(h []byte) { h[off+10] = 0 },
		"lanes 255":       func(h []byte) { h[off+10] = 255 },
		"magic":           func(h []byte) { h[0] = 'X' },
	}
	for name, mutate := range cases {
		tampered := append([]byte(nil), ct...)
		mutate(tampered)
		start := time.Now()
		_, err := DecryptByPassword([]byte(passwordTestPassword), tampered)
		require.Error(t, err, name)
		require.Less(t, time.Since(start), time.Second, "%s was not rejected before the KDF", name)
	}
}

// TestPasswordSecurity_ParamFloorAndBounds checks the caller-selected parameter
// floor and ceiling. Regression for issue #54.
func TestPasswordSecurity_ParamFloorAndBounds(t *testing.T) {
	t.Parallel()

	for name, p := range map[string]PasswordKDFParams{
		"time below floor":   {Time: 1, MemoryKiB: 64 * 1024, Parallelism: 1},
		"memory below floor": {Time: 3, MemoryKiB: 1024, Parallelism: 1},
		"time above bound":   {Time: 11, MemoryKiB: 64 * 1024, Parallelism: 1},
		"memory above bound": {Time: 3, MemoryKiB: 512 * 1024, Parallelism: 1},
		"lanes zero":         {Time: 3, MemoryKiB: 64 * 1024, Parallelism: 0},
		"lanes above bound":  {Time: 3, MemoryKiB: 64 * 1024, Parallelism: 17},
	} {
		_, err := NewPasswordEncryptor([]byte(passwordTestPassword), WithPasswordKDFParams(p))
		require.Error(t, err, name)
	}

	_, err := NewPasswordEncryptor(nil, fastPasswordKDF())
	require.Error(t, err, "empty password")

	enc, err := NewPasswordEncryptor([]byte(passwordTestPassword), fastPasswordKDF())
	require.NoError(t, err)
	enc.count.Store(passwordEncryptorMaxMessages)
	_, err = enc.Encrypt([]byte("x"))
	require.Error(t, err, "nonce budget exhausted")

	var nilEnc *PasswordEncryptor
	_, err = nilEnc.Encrypt([]byte("x"))
	require.Error(t, err)
}

// TestPasswordSecurity_DefaultParams runs one real-default (RFC 9106 second
// recommendation) round trip. Regression for issue #54.
func TestPasswordSecurity_DefaultParams(t *testing.T) {
	t.Parallel()

	ct, err := EncryptByPassword([]byte(passwordTestPassword), []byte("payload"))
	require.NoError(t, err)

	params, err := parsePasswordHeader(ct)
	require.NoError(t, err)
	require.Equal(t, DefaultPasswordKDFParams(), params)
	require.Equal(t, PasswordKDFParams{Time: 3, MemoryKiB: 64 * 1024, Parallelism: 4}, params)

	got, err := DecryptByPassword([]byte(passwordTestPassword), ct)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), got)
}
