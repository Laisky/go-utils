package crypto

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"testing"
	"testing/iotest"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestSecurity73RFC8032Vector tests section 7.3's Ed25519ph vector, also present
// in Go's crypto/ed25519/ed25519_test.go TestSignVerifyHashed.
func TestSecurity73RFC8032Vector(t *testing.T) {
	seed, err := hex.DecodeString("833fe62409237b9d62ec77587520911e9a759cec1d19755b7da901b96dca3d42")
	require.NoError(t, err)
	expectedPublic, err := hex.DecodeString("ec172b93ad5e563bf4932c70e1245034c35467ef2efd4d64ebf819683467e2bf")
	require.NoError(t, err)
	expectedSignature, err := hex.DecodeString("98a70222f0b8121aa9d30f813d683f809e462b469c7ff87639499bb94e6dae4131f85042463c2a355a2003d062adf5aaa10b8c61e636062aaad11c2a26083406")
	require.NoError(t, err)
	private := ed25519.NewKeyFromSeed(seed)
	public := private.Public().(ed25519.PublicKey)
	require.Equal(t, expectedPublic, []byte(public))
	signature, err := SignByEd25519ph(private, bytes.NewBufferString("abc"))
	require.NoError(t, err)
	require.Equal(t, expectedSignature, signature)
	legacy, err := SignByEd25519LegacySHA512(private, bytes.NewBufferString("abc"))
	require.NoError(t, err)
	require.Equal(t, "dc2a4459e7369633a52b1bf277839a00201009a3efbf3ecb69bea2186c26b58909351fc9ac90b3ecfdfbc7c66431e0303dca179c138ac17ad9bef1177331a704", hex.EncodeToString(legacy))
	require.Error(t, VerifyByEd25519ph(public, bytes.NewBufferString("abc"), legacy))
	require.NoError(t, VerifyByEd25519ph(public, bytes.NewBufferString("abc"), expectedSignature))
	digest := sha512.Sum512([]byte("abc"))
	require.NoError(t, ed25519.VerifyWithOptions(public, digest[:], signature, &ed25519.Options{Hash: crypto.SHA512}))
	require.Error(t, VerifyByEd25519ph(public, bytes.NewBufferString("changed"), expectedSignature))
}

// TestSecurity73ProtocolSeparation preserves legacy data without downgrade fallback.
func TestSecurity73ProtocolSeparation(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	message := []byte("synthetic protocol separation fixture")
	legacy, err := SignByEd25519WithSHA512(private, bytes.NewReader(message))
	require.NoError(t, err)
	standard, err := SignByEd25519ph(private, bytes.NewReader(message))
	require.NoError(t, err)
	require.NotEqual(t, legacy, standard)
	require.NoError(t, VerifyByEd25519WithSHA512(public, bytes.NewReader(message), legacy))
	explicitLegacy, err := SignByEd25519LegacySHA512(private, bytes.NewReader(message))
	require.NoError(t, err)
	require.Equal(t, legacy, explicitLegacy)
	require.NoError(t, VerifyByEd25519LegacySHA512(public, bytes.NewReader(message), legacy))
	require.Error(t, VerifyByEd25519LegacySHA512(public, bytes.NewReader(message), standard))
	require.NoError(t, VerifyByEd25519ph(public, bytes.NewReader(message), standard))
	require.Error(t, VerifyByEd25519ph(public, bytes.NewReader(message), legacy))
	require.Error(t, VerifyByEd25519WithSHA512(public, bytes.NewReader(message), standard))
	digest := sha512.Sum512(message)
	stdSignature, err := private.Sign(rand.Reader, digest[:], &ed25519.Options{Hash: crypto.SHA512})
	require.NoError(t, err)
	require.Equal(t, stdSignature, standard)
	require.NoError(t, VerifyByEd25519ph(public, bytes.NewReader(message), stdSignature))
}

// TestSecurity73InvalidInput checks malformed keys, signatures, and reader failures.
func TestSecurity73InvalidInput(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	for _, size := range []int{0, 1, 31, 33, 63, 65} {
		require.NotPanics(t, func() {
			_, err := SignByEd25519ph(make(ed25519.PrivateKey, size), bytes.NewReader(nil))
			require.Error(t, err)
		})
		require.NotPanics(t, func() {
			err := VerifyByEd25519ph(make(ed25519.PublicKey, size), bytes.NewReader(nil), make([]byte, 64))
			require.Error(t, err)
		})
	}
	require.Error(t, VerifyByEd25519ph(public, bytes.NewReader(nil), nil))
	_, err = SignByEd25519ph(private, nil)
	require.Error(t, err)
	synthetic := errors.New("synthetic reader failure")
	_, err = SignByEd25519ph(private, iotest.ErrReader(synthetic))
	require.ErrorIs(t, err, synthetic)
	err = VerifyByEd25519ph(public, iotest.ErrReader(synthetic), make([]byte, 64))
	require.ErrorIs(t, err, synthetic)
	require.NotPanics(t, func() { _, err := SignByEd25519WithSHA512(nil, bytes.NewReader(nil)); require.Error(t, err) })
	require.NotPanics(t, func() { err := VerifyByEd25519WithSHA512(nil, bytes.NewReader(nil), nil); require.Error(t, err) })
}

// TestSecurity73StreamingAndContext checks empty and fragmented input, context isolation, and wrong keys.
func TestSecurity73StreamingAndContext(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	wrongPublic, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	for _, message := range [][]byte{nil, []byte("fragmented synthetic message")} {
		signature, err := SignByEd25519ph(private, iotest.OneByteReader(bytes.NewReader(message)))
		require.NoError(t, err)
		require.NoError(t, VerifyByEd25519ph(public, iotest.DataErrReader(bytes.NewReader(message)), signature))
		require.Error(t, VerifyByEd25519ph(wrongPublic, bytes.NewReader(message), signature))
		digest := sha512.Sum512(message)
		contextual, err := private.Sign(rand.Reader, digest[:], &ed25519.Options{Hash: crypto.SHA512, Context: "other protocol"})
		require.NoError(t, err)
		require.Error(t, VerifyByEd25519ph(public, bytes.NewReader(message), contextual))
	}
}

// TestSecurity73ReaderAndLengthGuards checks every public format selector before
// reading a message, including seed-sized private inputs and short signatures.
func TestSecurity73ReaderAndLengthGuards(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	for _, sign := range []func(ed25519.PrivateKey, io.Reader) ([]byte, error){SignByEd25519ph, SignByEd25519LegacySHA512, SignByEd25519WithSHA512} {
		for _, size := range []int{0, 1, 32, 63, 65} {
			require.NotPanics(t, func() {
				signature, err := sign(make(ed25519.PrivateKey, size), bytes.NewReader(nil))
				require.Error(t, err)
				require.Nil(t, signature)
			})
		}
		signature, err := sign(private, nil)
		require.Error(t, err)
		require.Nil(t, signature)
	}
	synthetic := errors.New("synthetic guarded reader failure")
	for _, verify := range []func(ed25519.PublicKey, io.Reader, []byte) error{VerifyByEd25519ph, VerifyByEd25519LegacySHA512, VerifyByEd25519WithSHA512} {
		require.Error(t, verify(public, nil, make([]byte, ed25519.SignatureSize)))
		require.ErrorIs(t, verify(public, iotest.ErrReader(synthetic), make([]byte, ed25519.SignatureSize)), synthetic)
		for _, size := range []int{0, 1, 63, 65, 128} {
			require.Error(t, verify(public, iotest.ErrReader(synthetic), make([]byte, size)))
		}
	}
}
