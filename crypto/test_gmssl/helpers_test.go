// Package testgmssl cross-checks the go-utils Tongsuo wrapper against independent
// SM2/SM3/SM4 implementations: the GmSSL C library (through GmSSL-Go) and the
// Tongsuo C library (through tongsuo-go-sdk). See README.md for prerequisites.
package testgmssl

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gmssl "github.com/GmSSL/GmSSL-Go"
	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// HKDF info labels and envelope constants copied verbatim from the wire format
// documented in crypto/smtongsuo.md. The tests rebuild the formats from these
// documented values only, so a silent format change in go-utils breaks them.
const (
	// docSm4EnvelopeEncKeyInfo is the documented HKDF info label of the v1 envelope SM4 key.
	docSm4EnvelopeEncKeyInfo = "github.com/Laisky/go-utils/crypto sm4-cbc-hmac-sha256 envelope v1 encryption key"
	// docSm4EnvelopeMACKeyInfo is the documented HKDF info label of the v1 envelope MAC key.
	docSm4EnvelopeMACKeyInfo = "github.com/Laisky/go-utils/crypto sm4-cbc-hmac-sha256 envelope v1 mac key"
	// docSm4BasicMACKeyInfo is the documented HKDF info label of the basic API MAC key.
	docSm4BasicMACKeyInfo = "github.com/Laisky/go-utils/crypto sm4-cbc basic iv-and-ciphertext v1 mac key"
	// docSm4EnvelopeMagic is the documented v1 envelope magic prefix.
	docSm4EnvelopeMagic = "GUS4"
	// docSm4EnvelopeVersion is the documented v1 envelope version byte.
	docSm4EnvelopeVersion byte = 0x01
	// docSm4IVSize is the SM4-CBC IV size in bytes.
	docSm4IVSize = 16
	// docSm4TagSize is the HMAC-SHA256 tag size in bytes.
	docSm4TagSize = sha256.Size
)

// tongsuoExePath returns the path of the tongsuo binary found on PATH. It takes
// the calling test t, skips t when tongsuo is not installed, fails t on any
// other lookup error, and returns the resolved executable path.
func tongsuoExePath(t *testing.T) string {
	t.Helper()
	exePath, err := exec.LookPath("tongsuo")
	if err != nil {
		require.ErrorIs(t, err, exec.ErrNotFound)
		t.Skip("tongsuo binary not found in PATH")
	}

	return exePath
}

// newTongsuo creates a go-utils Tongsuo wrapper around the tongsuo binary on
// PATH. It takes the calling test t, skips t when tongsuo is missing, fails t
// when the wrapper cannot be created, and returns the wrapper.
func newTongsuo(t *testing.T) *gcrypto.Tongsuo {
	t.Helper()
	ins, err := gcrypto.NewTongsuo(tongsuoExePath(t))
	require.NoError(t, err)

	return ins
}

// randomBytes returns n cryptographically random bytes. It takes the calling
// test t and a non-negative length n, fails t if randomness is unavailable, and
// returns an empty, non-nil slice when n is zero.
func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	_, err := rand.Read(buf)
	require.NoError(t, err)

	return buf
}

// flipByte returns a copy of b with the lowest bit of the byte at index i
// inverted. It takes a slice b and an index i inside it, and never modifies b.
func flipByte(b []byte, i int) []byte {
	out := bytes.Clone(b)
	out[i] ^= 0x01

	return out
}

// flipLastByte returns a copy of the nonempty slice b whose last byte has its
// lowest bit inverted, leaving b untouched.
func flipLastByte(b []byte) []byte {
	return flipByte(b, len(b)-1)
}

// pemBlockBytes decodes the first PEM block in data, skipping any leading
// non-PEM text. It takes the calling test t and the PEM data, fails t when no
// block is present, and returns the decoded block bytes.
func pemBlockBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(data)
	require.NotNil(t, block, "no PEM block found")

	return block.Bytes
}

// gmsslSm4Cbc runs SM4-CBC with PKCS#7 padding through GmSSL. It takes the
// calling test t, encrypt to select the direction, the 16-byte key and iv, and
// the input data; it fails t on any GmSSL error and returns the output bytes.
func gmsslSm4Cbc(t *testing.T, encrypt bool, key, iv, data []byte) []byte {
	t.Helper()
	c, err := gmssl.NewSm4Cbc(key, iv, encrypt)
	require.NoError(t, err)

	var out []byte
	// GmSSL-Go dereferences &data[0], so empty input must skip Update.
	if len(data) > 0 {
		out, err = c.Update(data)
		require.NoError(t, err)
	}
	last, err := c.Finish()
	require.NoError(t, err)

	return append(out, last...)
}

// gmsslSm2Verify verifies a DER SM2/SM3 signature with GmSSL using the default
// SM2 user ID. It takes the calling test t, a PEM SubjectPublicKeyInfo, the
// signed message and the signature; it builds a fresh verifier on every call
// (a GmSSL verifier is single-use), fails t on setup errors, and returns
// whether the signature is valid.
func gmsslSm2Verify(t *testing.T, pubkeyPem, msg, signature []byte) bool {
	t.Helper()
	pubkeyPath := filepath.Join(t.TempDir(), "pubkey.pem")
	require.NoError(t, os.WriteFile(pubkeyPath, pubkeyPem, 0o600))
	pubkey, err := gmssl.ImportSm2PublicKeyInfoPem(pubkeyPath)
	require.NoError(t, err)

	verifier, err := gmssl.NewSm2Signature(pubkey, gmssl.Sm2DefaultId, false)
	require.NoError(t, err)
	require.NoError(t, verifier.Update(msg))
	// GmSSL-Go dereferences &signature[0], so an empty signature is invalid here.
	if len(signature) == 0 {
		return false
	}

	return verifier.Verify(signature)
}

// docHKDFSha256 derives length bytes from key with HKDF-SHA256, an empty salt,
// and the given info label, using only the Go standard library. It takes the
// calling test t, the input key material, the info label and the output
// length, fails t on error, and returns the derived key.
func docHKDFSha256(t *testing.T, key []byte, info string, length int) []byte {
	t.Helper()
	derived, err := hkdf.Key(sha256.New, key, nil, info, length)
	require.NoError(t, err)

	return derived
}

// docHMACSha256 returns HMAC-SHA256 under macKey over the concatenation of parts.
func docHMACSha256(macKey []byte, parts ...[]byte) []byte {
	mac := hmac.New(sha256.New, macKey)
	for _, part := range parts {
		mac.Write(part)
	}

	return mac.Sum(nil)
}
