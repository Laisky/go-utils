package crypto

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"testing"

	"github.com/emmansun/gmsm/sm4"
	"github.com/stretchr/testify/require"
)

// sm4TestRawCbcEncrypt encrypts already block-aligned data with SM4-CBC without
// adding padding. It is an independent test reimplementation used to build
// legacy envelopes and invalid-padding fixtures, and returns the ciphertext.
func sm4TestRawCbcEncrypt(t *testing.T, key, iv, aligned []byte) []byte {
	t.Helper()
	require.Zero(t, len(aligned)%sm4.BlockSize)
	block, err := sm4.NewCipher(key)
	require.NoError(t, err)

	out := make([]byte, len(aligned))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, aligned)

	return out
}

// sm4TestPkcs7Pad returns plaintext with PKCS#7 padding to the SM4 block size,
// reimplemented independently of the production code.
func sm4TestPkcs7Pad(plaintext []byte) []byte {
	padLen := sm4.BlockSize - len(plaintext)%sm4.BlockSize

	return append(bytes.Clone(plaintext), bytes.Repeat([]byte{byte(padLen)}, padLen)...)
}

// sm4TestLegacyEncrypt reproduces the pre-#45 EncryptBySm4Cbc output format,
// iv || SM4-CBC(key, iv, plaintext) || HMAC-SHA256(key, ciphertext), and returns it.
func sm4TestLegacyEncrypt(t *testing.T, key, plaintext []byte) []byte {
	t.Helper()
	iv, err := Salt(16)
	require.NoError(t, err)
	ciphertext := sm4TestRawCbcEncrypt(t, key, iv, sm4TestPkcs7Pad(plaintext))

	return sm4TestLegacyCombine(key, iv, ciphertext)
}

// sm4TestLegacyCombine builds a legacy combined envelope from iv and ciphertext
// with the legacy HMAC-SHA256(key, ciphertext) tag, and returns it.
func sm4TestLegacyCombine(key, iv, ciphertext []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(ciphertext)

	out := append(bytes.Clone(iv), ciphertext...)

	return append(out, mac.Sum(nil)...)
}

// sm4TestPlaintextLengths lists plaintext lengths around SM4 block boundaries.
var sm4TestPlaintextLengths = []int{0, 1, 15, 16, 17, 31, 32, 33, 48, 1000}

// TestTongsuoSm4CbcRoundTrips verifies envelope and basic API round trips for
// plaintext lengths around block boundaries, including empty plaintext, and the
// resulting sizes; regression for issue #45.
func TestTongsuoSm4CbcRoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ins := newSm4TestTongsuo(t)

	for _, n := range sm4TestPlaintextLengths {
		key, plaintext := sm4TestKeyAndPlaintext(t, n)
		paddedLen := (n/16 + 1) * 16

		combined, err := ins.EncryptBySm4Cbc(ctx, key, plaintext)
		require.NoError(t, err)
		require.Len(t, combined, sm4EnvelopeHeaderSize+sm4IVSize+paddedLen+sm4MACSize)
		got, err := ins.DecryptBySm4Cbc(ctx, key, combined)
		require.NoError(t, err)
		require.Equal(t, len(plaintext), len(got))
		require.True(t, bytes.Equal(plaintext, got), "plaintext length %d", n)

		again, err := ins.EncryptBySm4Cbc(ctx, key, plaintext)
		require.NoError(t, err)
		require.NotEqual(t, combined, again, "each envelope must use a fresh IV")

		iv, err := Salt(16)
		require.NoError(t, err)
		ciphertext, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.Len(t, ciphertext, paddedLen)
		require.Len(t, tag, sm4MACSize)
		require.Equal(t, sm4TestRawCbcEncrypt(t, key, iv, sm4TestPkcs7Pad(plaintext)), ciphertext,
			"basic ciphertext must be standard SM4-CBC-PKCS7 under the raw key")

		got, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, tag)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, got), "plaintext length %d", n)
		got, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, nil)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, got), "plaintext length %d without mac", n)
	}
}

// TestTongsuoSm4CbcKeySeparation verifies that the envelope does not encrypt or
// authenticate under the raw caller key, and that the basic API MAC differs from
// both the legacy MAC and a raw-key MAC over iv || ciphertext; regression for issue #45.
func TestTongsuoSm4CbcKeySeparation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ins := newSm4TestTongsuo(t)
	key, plaintext := sm4TestKeyAndPlaintext(t, 64)

	combined, err := ins.EncryptBySm4Cbc(ctx, key, plaintext)
	require.NoError(t, err)
	iv := combined[sm4EnvelopeHeaderSize : sm4EnvelopeHeaderSize+sm4IVSize]
	ciphertext := combined[sm4EnvelopeHeaderSize+sm4IVSize : len(combined)-sm4MACSize]
	rawKeyPlain, rawKeyErr := ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, nil)
	if rawKeyErr == nil {
		require.NotEqual(t, plaintext, rawKeyPlain, "envelope ciphertext must not be encrypted under the raw key")
	}

	encKey, macKey, err := deriveSm4EnvelopeKeys(key)
	require.NoError(t, err)
	require.NotEqual(t, key, encKey)
	require.NotEqual(t, encKey, macKey[:16])
	basicMACKey, err := deriveSm4Key(key, sm4BasicMACKeyInfo, sm4MACSize)
	require.NoError(t, err)
	require.NotEqual(t, macKey, basicMACKey)

	basicCT, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
	require.NoError(t, err)
	legacyTag := sm4TestLegacyCombine(key, iv, basicCT)[sm4IVSize+len(basicCT):]
	require.NotEqual(t, legacyTag, tag)
	require.NotEqual(t, sm4HMAC(key, iv, basicCT), tag)
	require.Equal(t, sm4HMAC(basicMACKey, iv, basicCT), tag)
}

// TestTongsuoSm4CbcLegacyDecoders verifies the explicit legacy migration path:
// legacy envelopes and legacy basic MACs decrypt only through the named legacy
// functions, the current decoders reject them without silent fallback, and the
// legacy decoders reject current-format input; regression for issue #45.
func TestTongsuoSm4CbcLegacyDecoders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ins := newSm4TestTongsuo(t)

	for _, n := range sm4TestPlaintextLengths {
		key, plaintext := sm4TestKeyAndPlaintext(t, n)
		legacy := sm4TestLegacyEncrypt(t, key, plaintext)

		got, err := ins.DecryptBySm4CbcLegacy(ctx, key, legacy)
		require.NoError(t, err, "legacy envelope of %d bytes", n)
		require.True(t, bytes.Equal(plaintext, got))

		got, err = ins.DecryptBySm4Cbc(ctx, key, legacy)
		require.ErrorIs(t, err, ErrSm4CbcMalformedCiphertext, "current decoder must reject legacy input")
		require.Nil(t, got)

		// Migration: decrypt legacy, then re-encrypt with the current envelope.
		migrated, err := ins.EncryptBySm4Cbc(ctx, key, plaintext)
		require.NoError(t, err)
		got, err = ins.DecryptBySm4CbcLegacy(ctx, key, migrated)
		require.ErrorIs(t, err, ErrSm4CbcMalformedCiphertext, "legacy decoder must reject v1 input")
		require.ErrorContains(t, err, "DecryptBySm4Cbc")
		require.Nil(t, got)

		iv := legacy[:sm4IVSize]
		ciphertext := legacy[sm4IVSize : len(legacy)-sm4MACSize]
		legacyTag := legacy[len(legacy)-sm4MACSize:]
		got, err = ins.DecryptBySm4CbcBaiscLegacy(ctx, key, ciphertext, iv, legacyTag)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, got))

		got, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, legacyTag)
		require.ErrorIs(t, err, ErrSm4CbcHMACMismatch, "current basic decoder must reject legacy MACs")
		require.Nil(t, got)

		_, currentTag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		got, err = ins.DecryptBySm4CbcBaiscLegacy(ctx, key, ciphertext, iv, currentTag)
		require.ErrorIs(t, err, ErrSm4CbcHMACMismatch, "legacy basic decoder must reject current MACs")
		require.Nil(t, got)
	}

	t.Run("legacy basic decoder requires a mac", func(t *testing.T) {
		t.Parallel()
		key, plaintext := sm4TestKeyAndPlaintext(t, 20)
		legacy := sm4TestLegacyEncrypt(t, key, plaintext)
		got, err := ins.DecryptBySm4CbcBaiscLegacy(ctx, key, legacy[16:len(legacy)-32], legacy[:16], nil)
		require.ErrorContains(t, err, "hmac should be 32 bytes")
		require.Nil(t, got)
	})

	t.Run("legacy envelope malformed lengths", func(t *testing.T) {
		t.Parallel()
		key, plaintext := sm4TestKeyAndPlaintext(t, 20)
		legacy := sm4TestLegacyEncrypt(t, key, plaintext)
		for _, n := range []int{0, 1, 48, 63, len(legacy) - 1} {
			got, err := ins.DecryptBySm4CbcLegacy(ctx, key, legacy[:n])
			require.ErrorIs(t, err, ErrSm4CbcMalformedCiphertext, "length %d", n)
			require.Nil(t, got)
		}
	})
}

// TestTongsuoSm4CbcLegacyDecoderDoesNotAuthenticateIV pins the documented
// limitation of the legacy decoder: it accepts a modified IV and returns a
// first block changed by the same delta, which is why it is restricted to
// explicit migration; regression for issue #45.
func TestTongsuoSm4CbcLegacyDecoderDoesNotAuthenticateIV(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ins := newSm4TestTongsuo(t)
	key, plaintext := sm4TestKeyAndPlaintext(t, 48)

	legacy := sm4TestLegacyEncrypt(t, key, plaintext)
	legacy[0] ^= 0x01

	got, err := ins.DecryptBySm4CbcLegacy(ctx, key, legacy)
	require.NoError(t, err)
	expected := bytes.Clone(plaintext)
	expected[0] ^= 0x01
	require.Equal(t, expected, got)
}

// TestTongsuoSm4CbcStrictPadding verifies that basic decryption without a MAC
// rejects every malformed PKCS#7 padding with ErrSm4CbcBadDecrypt and no
// plaintext, while all valid padding lengths are accepted.
func TestTongsuoSm4CbcStrictPadding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ins := newSm4TestTongsuo(t)
	key, err := Salt(16)
	require.NoError(t, err)
	iv, err := Salt(16)
	require.NoError(t, err)

	invalidLastBlocks := [][]byte{
		append(bytes.Repeat([]byte{'a'}, 15), 0x00),
		append(bytes.Repeat([]byte{'a'}, 15), 0x11),
		append(bytes.Repeat([]byte{'a'}, 15), 0xff),
		append(bytes.Repeat([]byte{'a'}, 14), 0x03, 0x02),
		append(append(bytes.Repeat([]byte{'a'}, 12), 0x04, 0x05), 0x04, 0x04),
		append([]byte{0x0f}, bytes.Repeat([]byte{0x10}, 15)...),
	}
	for i, last := range invalidLastBlocks {
		aligned := append(bytes.Repeat([]byte{'p'}, 16), last...)
		ciphertext := sm4TestRawCbcEncrypt(t, key, iv, aligned)

		got, err := ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, nil)
		require.ErrorIs(t, err, ErrSm4CbcBadDecrypt, "invalid padding case %d", i)
		require.Nil(t, got)
	}

	for padLen := 1; padLen <= 16; padLen++ {
		last := append(bytes.Repeat([]byte{'v'}, 16-padLen), bytes.Repeat([]byte{byte(padLen)}, padLen)...)
		ciphertext := sm4TestRawCbcEncrypt(t, key, iv, last)

		got, err := ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, nil)
		require.NoError(t, err, "valid padding length %d", padLen)
		require.Equal(t, last[:16-padLen], got)
	}
}

// TestPkcs7PaddingLen verifies the constant-time PKCS#7 padding validator on
// valid and invalid inputs, including lengths that are not block aligned.
func TestPkcs7PaddingLen(t *testing.T) {
	t.Parallel()

	for padLen := 1; padLen <= 16; padLen++ {
		data := append(bytes.Repeat([]byte{0xaa}, 32-padLen), bytes.Repeat([]byte{byte(padLen)}, padLen)...)
		got, ok := pkcs7PaddingLen(data, 16)
		require.True(t, ok, "padding %d", padLen)
		require.Equal(t, padLen, got)
	}

	for _, data := range [][]byte{
		nil,
		bytes.Repeat([]byte{0x01}, 15),
		bytes.Repeat([]byte{0x01}, 17),
		bytes.Repeat([]byte{0x00}, 16),
		bytes.Repeat([]byte{0x11}, 32),
		append(bytes.Repeat([]byte{0x01}, 15), 0x02),
	} {
		got, ok := pkcs7PaddingLen(data, 16)
		require.False(t, ok, "data %x", data)
		require.Zero(t, got)
	}
}
