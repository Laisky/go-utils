package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOtpMnemonicBehavior groups all behavioral tests for OTP and Mnemonic
// functionality that are not already covered by existing test files.
func TestOtpMnemonicBehavior(t *testing.T) {
	t.Parallel()

	// -------------------------------------------------------------------
	// OTP tests
	// -------------------------------------------------------------------

	t.Run("NewTOTP_EmptySecret", func(t *testing.T) {
		t.Parallel()
		_, err := NewTOTP(OTPArgs{
			Base32Secret: "",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "secret")
	})

	t.Run("NewTOTP_UnsupportedAlgorithm", func(t *testing.T) {
		t.Parallel()
		_, err := NewTOTP(OTPArgs{
			Base32Secret: Base32Secret([]byte("testsecret")),
			Algorithm:    OTPAlgorithm("md5"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupport")
	})

	t.Run("ParseOTPUri_HOTP", func(t *testing.T) {
		t.Parallel()
		uri := "otpauth://hotp/issuer:account?secret=JBSWY3DPEHPK3PXP&counter=42&digits=6&period=30"
		arg, err := ParseOTPUri(uri)
		require.NoError(t, err)
		require.Equal(t, OTPTypeHOTP, arg.OtpType)
		require.Equal(t, 42, arg.InitialCount)
		require.Equal(t, "JBSWY3DPEHPK3PXP", arg.Base32Secret)
		require.Equal(t, "account", arg.AccountName)
		require.Equal(t, "issuer", arg.IssuerName)
	})

	t.Run("ParseOTPUri_MalformedURI", func(t *testing.T) {
		t.Parallel()
		// url.Parse is very lenient, but a completely garbage string with
		// invalid percent-encoding should fail.
		_, err := ParseOTPUri("://%%%not-a-uri")
		require.Error(t, err)
	})

	t.Run("ParseOTPUri_DigitsZero", func(t *testing.T) {
		t.Parallel()
		uri := "otpauth://totp/test:test?secret=JBSWY3DPEHPK3PXP&digits=0"
		arg, err := ParseOTPUri(uri)
		require.NoError(t, err)
		require.Equal(t, uint(6), arg.Digits, "digits=0 should default to 6")
	})

	t.Run("ParseOTPUri_PeriodZero", func(t *testing.T) {
		t.Parallel()
		uri := "otpauth://totp/test:test?secret=JBSWY3DPEHPK3PXP&period=0"
		arg, err := ParseOTPUri(uri)
		require.NoError(t, err)
		require.Equal(t, uint(30), arg.PeriodSecs, "period=0 should default to 30")
	})

	t.Run("Base32Secret_Encoding", func(t *testing.T) {
		t.Parallel()
		input := []byte("Hello!")
		got := Base32Secret(input)
		expected := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(input)
		require.Equal(t, expected, got)
		// Verify it decodes back correctly.
		decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(got)
		require.NoError(t, err)
		require.Equal(t, input, decoded)
	})

	t.Run("TOTP_KeyAt_Deterministic", func(t *testing.T) {
		t.Parallel()
		totp, err := NewTOTP(OTPArgs{
			Base32Secret: Base32Secret([]byte("deterministic")),
			Digits:       6,
			PeriodSecs:   30,
		})
		require.NoError(t, err)

		fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		key1 := totp.KeyAt(fixedTime)
		key2 := totp.KeyAt(fixedTime)
		require.Equal(t, key1, key2, "same time must produce same key")
		require.Len(t, key1, 6)
	})

	t.Run("TOTP_URI_Format", func(t *testing.T) {
		t.Parallel()
		secret := Base32Secret([]byte("uricheck"))
		totp, err := NewTOTP(OTPArgs{
			Base32Secret: secret,
			AccountName:  "user@example.com",
			IssuerName:   "MyApp",
			Digits:       8,
			PeriodSecs:   60,
		})
		require.NoError(t, err)

		uri := totp.URI()
		require.Contains(t, uri, "otpauth://totp/")
		require.Contains(t, uri, "secret="+secret)
		require.Contains(t, uri, "issuer=MyApp")
		require.Contains(t, uri, "period=60")
		require.Contains(t, uri, "digits=8")
	})

	// -------------------------------------------------------------------
	// Mnemonic tests (only scenarios NOT in mnemonic_test.go)
	// -------------------------------------------------------------------

	t.Run("BytesToMnemonic_MnemonicToBytes_Roundtrip", func(t *testing.T) {
		t.Parallel()

		sizes := []int{1, 32, 256}
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%dBytes", size), func(t *testing.T) {
				t.Parallel()
				data := make([]byte, size)
				_, err := rand.Read(data)
				require.NoError(t, err)

				mnemonic, err := BytesToMnemonic(data)
				require.NoError(t, err)
				require.NotEmpty(t, mnemonic)

				recovered, err := MnemonicToBytes(mnemonic)
				require.NoError(t, err)
				require.True(t, bytes.Equal(data, recovered),
					"roundtrip failed for %d bytes", size)
			})
		}
	})

	t.Run("BytesToMnemonic_EmptyInput", func(t *testing.T) {
		t.Parallel()
		_, err := BytesToMnemonic([]byte{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty")
	})

	t.Run("MnemonicToBytes_EmptyInput", func(t *testing.T) {
		t.Parallel()
		_, err := MnemonicToBytes("")
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty")
	})

	t.Run("MnemonicToBytes_TamperedWords", func(t *testing.T) {
		t.Parallel()
		data := []byte("tamper test data for checksum")
		mnemonic, err := BytesToMnemonic(data)
		require.NoError(t, err)

		words := strings.Fields(mnemonic)
		require.True(t, len(words) > 2, "need at least 3 words")

		// Swap a word to a different valid BIP39 word to break the checksum.
		original := words[1]
		if original == "abandon" {
			words[1] = "zoo"
		} else {
			words[1] = "abandon"
		}

		tampered := strings.Join(words, " ")
		_, err = MnemonicToBytes(tampered)
		require.Error(t, err, "tampered mnemonic should fail checksum verification")
	})

	t.Run("NewMnemonic_ValidBitSizes", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			bits      int
			wordCount int
		}{
			{128, 12},
			{256, 24},
		}

		for _, tc := range tests {
			t.Run(fmt.Sprintf("%dBits", tc.bits), func(t *testing.T) {
				t.Parallel()
				m, err := NewMnemonic(tc.bits)
				require.NoError(t, err)

				words := strings.Fields(m)
				require.Len(t, words, tc.wordCount)
				require.True(t, ValidateMnemonic(m))
			})
		}
	})

	t.Run("EntropyToMnemonic_MnemonicToEntropy_Roundtrip", func(t *testing.T) {
		t.Parallel()
		entropy := make([]byte, 16) // 128 bits
		_, err := rand.Read(entropy)
		require.NoError(t, err)

		mnemonic, err := EntropyToMnemonic(entropy)
		require.NoError(t, err)
		require.True(t, ValidateMnemonic(mnemonic))

		recovered, err := MnemonicToEntropy(mnemonic)
		require.NoError(t, err)
		require.Equal(t, entropy, recovered)
	})

	t.Run("MnemonicToSeed_ProducesSeed", func(t *testing.T) {
		t.Parallel()
		m, err := NewMnemonic(128)
		require.NoError(t, err)

		seed, err := MnemonicToSeed(m, "")
		require.NoError(t, err)
		require.Len(t, seed, 64, "seed should be 64 bytes (512 bits)")
	})

	t.Run("ValidateMnemonic_ValidAndInvalid", func(t *testing.T) {
		t.Parallel()

		m, err := NewMnemonic(128)
		require.NoError(t, err)
		require.True(t, ValidateMnemonic(m), "valid mnemonic should return true")

		require.False(t, ValidateMnemonic("this is not a valid mnemonic phrase at all"),
			"invalid mnemonic should return false")
		require.False(t, ValidateMnemonic(""),
			"empty mnemonic should return false")
	})

	t.Run("PrikeyToMnemonic_MnemonicToPrikey_Ed25519_NoPassphrase", func(t *testing.T) {
		t.Parallel()
		_, prikey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		mnemonic, err := PrikeyToMnemonic(prikey)
		require.NoError(t, err)
		require.NotEmpty(t, mnemonic)

		recovered, err := MnemonicToPrikey(mnemonic)
		require.NoError(t, err)

		recoveredEd, ok := recovered.(ed25519.PrivateKey)
		require.True(t, ok, "recovered key should be ed25519")
		require.True(t, bytes.Equal(prikey, recoveredEd),
			"recovered key should match original")
	})

	t.Run("PrikeyToMnemonic_MnemonicToPrikey_WithPassphrase", func(t *testing.T) {
		t.Parallel()
		_, prikey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		passphrase := "correct-horse-battery-staple"
		mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase(passphrase))
		require.NoError(t, err)

		// Decrypt with correct passphrase.
		recovered, err := MnemonicToPrikey(mnemonic, WithMnemonicPassphrase(passphrase))
		require.NoError(t, err)
		recoveredEd, ok := recovered.(ed25519.PrivateKey)
		require.True(t, ok)
		require.True(t, bytes.Equal(prikey, recoveredEd))

		// Decrypt with wrong passphrase should fail.
		_, err = MnemonicToPrikey(mnemonic, WithMnemonicPassphrase("wrong-passphrase"))
		require.Error(t, err, "wrong passphrase should fail decryption")
	})

	t.Run("MnemonicToPrikey_EncryptedWithoutPassphrase", func(t *testing.T) {
		t.Parallel()
		_, prikey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase("secret"))
		require.NoError(t, err)

		// Attempt to decrypt without providing a passphrase.
		_, err = MnemonicToPrikey(mnemonic)
		require.Error(t, err)
		require.Contains(t, err.Error(), "encrypted but no passphrase")
	})
}
