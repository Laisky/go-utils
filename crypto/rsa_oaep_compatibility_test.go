package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// rsaCompatibilityFixture records disposable ciphertext from both pre-change implementations.
type rsaCompatibilityFixture struct {
	SourceCommit         string `json:"source_commit"`
	EnvelopeSourceCommit string `json:"envelope_source_commit"`
	PrivateKey           []byte `json:"private_key_pkcs1"`
	Cases                []struct {
		Plaintext  []byte `json:"plaintext"`
		Ciphertext []byte `json:"ciphertext"`
		Envelope   []byte `json:"envelope"`
	} `json:"cases"`
}

// loadRSACompatibilityFixture loads a public test-only key and immutable interoperability fixtures.
func loadRSACompatibilityFixture(t *testing.T) (*rsa.PrivateKey, rsaCompatibilityFixture) {
	t.Helper()
	data, err := os.ReadFile("testdata/rsa_oaep_compatibility.json")
	require.NoError(t, err)
	var fixture rsaCompatibilityFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Equal(t, "042437b6015f5ba8f185ef91b05b2bd3a5a420f3", fixture.SourceCommit)
	require.Equal(t, "a389e7ad3e9103250acf5f5d5f7ec543a9522032", fixture.EnvelopeSourceCommit)
	key, err := x509.ParsePKCS1PrivateKey(fixture.PrivateKey)
	require.NoError(t, err)
	require.NoError(t, key.Validate())
	require.Len(t, fixture.Cases, 3)
	return key, fixture
}

// TestRSAOAEPHistoricalCiphertexts verifies ciphertext produced by the actual pre-#123 API remains readable.
func TestRSAOAEPHistoricalCiphertexts(t *testing.T) {
	key, fixture := loadRSACompatibilityFixture(t)
	for _, tc := range fixture.Cases {
		t.Run(strconv.Itoa(len(tc.Plaintext)), func(t *testing.T) {
			// The independent decoder verifies the fixture rather than trusting metadata.
			var control []byte
			for offset := 0; offset < len(tc.Ciphertext); offset += key.Size() {
				part, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, tc.Ciphertext[offset:offset+key.Size()], nil)
				require.NoError(t, err)
				control = append(control, part...)
			}
			require.True(t, bytes.Equal(tc.Plaintext, control))
			actual, err := RSADecryptByOAEP(key, tc.Ciphertext)
			require.NoError(t, err, "existing stored ciphertext must remain readable")
			require.True(t, bytes.Equal(tc.Plaintext, actual))
			if len(tc.Plaintext) == 0 {
				require.Nil(t, actual)
			}
		})
	}
}

// TestRSAOAEPHistoricalWriter requires original block counts, SHA-256 and nil-label interoperability.
func TestRSAOAEPHistoricalWriter(t *testing.T) {
	key, fixture := loadRSACompatibilityFixture(t)
	capacity := key.Size() - 2*sha256.Size - 2
	for _, tc := range fixture.Cases {
		t.Run(strconv.Itoa(len(tc.Plaintext)), func(t *testing.T) {
			ciphertext, err := RSAEncryptByOAEP(&key.PublicKey, tc.Plaintext)
			require.NoError(t, err)
			require.Len(t, ciphertext, ((len(tc.Plaintext)+capacity-1)/capacity)*key.Size())
			var decoded []byte
			for offset := 0; offset < len(ciphertext); offset += key.Size() {
				part, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, ciphertext[offset:offset+key.Size()], nil)
				require.NoError(t, err, "old readers must accept newly written ciphertext")
				decoded = append(decoded, part...)
			}
			require.True(t, bytes.Equal(tc.Plaintext, decoded))
			if len(tc.Plaintext) == 0 {
				require.Nil(t, ciphertext)
			}
		})
	}
}

// TestRSAOAEPHistoricalErrors preserves empty-input behavior and decryption error context without partial plaintext.
func TestRSAOAEPHistoricalErrors(t *testing.T) {
	key, fixture := loadRSACompatibilityFixture(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	ciphertext := fixture.Cases[len(fixture.Cases)-1].Ciphertext
	for name, input := range map[string][]byte{
		"partial_block":   ciphertext[:len(ciphertext)-1],
		"corrupted_block": append(bytes.Clone(ciphertext[:key.Size()]), make([]byte, key.Size())...),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := RSADecryptByOAEP(key, input)
			require.ErrorIs(t, err, rsa.ErrDecryption)
			require.ErrorContains(t, err, "decrypt chunk")
			require.Nil(t, out)
		})
	}
	out, err := RSADecryptByOAEP(other, ciphertext)
	require.ErrorIs(t, err, rsa.ErrDecryption)
	require.ErrorContains(t, err, "decrypt chunk")
	require.Nil(t, out)
}
