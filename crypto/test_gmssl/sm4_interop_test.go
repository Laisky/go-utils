package testgmssl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// sm4TestLengths lists plaintext sizes covering empty input, partial and exact
// SM4 blocks, and a 1 MiB buffer.
var sm4TestLengths = []int{0, 1, 15, 16, 17, 1 << 20}

// TestTongsuo_EncryptBySm4CbcBaisc verifies that the basic SM4-CBC API is plain
// SM4-CBC with PKCS#7 padding interoperable with GmSSL in both directions, and
// that its tag is exactly the documented HMAC-SHA256 over iv || ciphertext under
// the HKDF-derived basic MAC key (crypto/smtongsuo.md). GmSSL ciphertext is
// accepted without a tag and with a tag computed independently from the docs,
// and that tag is rejected once the IV changes.
func TestTongsuo_EncryptBySm4CbcBaisc(t *testing.T) {
	t.Parallel()
	ins := newTongsuo(t)

	for _, n := range sm4TestLengths {
		t.Run(fmt.Sprintf("len=%d", n), func(t *testing.T) {
			key := randomBytes(t, 16)
			iv := randomBytes(t, docSm4IVSize)
			plaintext := randomBytes(t, n)
			macKey := docHKDFSha256(t, key, docSm4BasicMACKeyInfo, docSm4TagSize)

			t.Run("go-utils -> gmssl", func(t *testing.T) {
				ciphertext, tag, err := ins.EncryptBySm4CbcBaisc(t.Context(), key, plaintext, iv)
				require.NoError(t, err)

				require.Equal(t, plaintext, gmsslSm4Cbc(t, false, key, iv, ciphertext))
				require.Equal(t, docHMACSha256(macKey, iv, ciphertext), tag)
			})

			t.Run("gmssl -> go-utils", func(t *testing.T) {
				ctx := t.Context()
				ciphertext := gmsslSm4Cbc(t, true, key, iv, plaintext)

				got, err := ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, nil)
				require.NoError(t, err)
				require.Equal(t, plaintext, got)

				tag := docHMACSha256(macKey, iv, ciphertext)
				got, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, tag)
				require.NoError(t, err)
				require.Equal(t, plaintext, got)

				_, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, flipByte(iv, 0), tag)
				require.ErrorIs(t, err, gcrypto.ErrSm4CbcHMACMismatch)
			})
		})
	}
}

// TestTongsuo_Sm4CbcEnvelopeInterop verifies that the documented v1 SM4-CBC
// envelope, "GUS4" || 0x01 || iv(16) || ciphertext || HMAC-SHA256 tag(32), can
// be produced and consumed by an independent implementation built only from the
// crypto/smtongsuo.md description: GmSSL SM4-CBC plus standard-library
// HKDF-SHA256 and HMAC-SHA256. Envelopes from EncryptBySm4Cbc decrypt with
// GmSSL, envelopes built with GmSSL decrypt with DecryptBySm4Cbc, and a
// modified IV is rejected.
func TestTongsuo_Sm4CbcEnvelopeInterop(t *testing.T) {
	t.Parallel()
	ins := newTongsuo(t)
	header := append([]byte(docSm4EnvelopeMagic), docSm4EnvelopeVersion)

	for _, n := range sm4TestLengths {
		t.Run(fmt.Sprintf("len=%d", n), func(t *testing.T) {
			key := randomBytes(t, 16)
			plaintext := randomBytes(t, n)
			encKey := docHKDFSha256(t, key, docSm4EnvelopeEncKeyInfo, 16)
			macKey := docHKDFSha256(t, key, docSm4EnvelopeMACKeyInfo, docSm4TagSize)

			t.Run("go-utils -> gmssl", func(t *testing.T) {
				envelope, err := ins.EncryptBySm4Cbc(t.Context(), key, plaintext)
				require.NoError(t, err)

				minLen := len(header) + docSm4IVSize + 16 + docSm4TagSize
				require.GreaterOrEqual(t, len(envelope), minLen)
				require.Equal(t, header, envelope[:len(header)])

				body := envelope[:len(envelope)-docSm4TagSize]
				iv := body[len(header) : len(header)+docSm4IVSize]
				ciphertext := body[len(header)+docSm4IVSize:]
				require.Equal(t, docHMACSha256(macKey, body), envelope[len(body):])
				require.Equal(t, plaintext, gmsslSm4Cbc(t, false, encKey, iv, ciphertext))
			})

			t.Run("gmssl -> go-utils", func(t *testing.T) {
				ctx := t.Context()
				iv := randomBytes(t, docSm4IVSize)
				ciphertext := gmsslSm4Cbc(t, true, encKey, iv, plaintext)

				body := append(append(append([]byte{}, header...), iv...), ciphertext...)
				envelope := append(body, docHMACSha256(macKey, body)...)

				got, err := ins.DecryptBySm4Cbc(ctx, key, envelope)
				require.NoError(t, err)
				require.Equal(t, plaintext, got)

				_, err = ins.DecryptBySm4Cbc(ctx, key, flipByte(envelope, len(header)))
				require.ErrorIs(t, err, gcrypto.ErrSm4CbcHMACMismatch)
			})
		})
	}
}
