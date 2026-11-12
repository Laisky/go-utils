package crypto

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"
)

// mnemonicCompatibilityFixture loads immutable synthetic pre-149 wire data and
// returns its private key, passphrase, ciphertext and DER to the calling test.
func mnemonicCompatibilityFixture(t *testing.T) (crypto.PrivateKey, string, []byte, []byte) {
	t.Helper()
	content, err := os.ReadFile("testdata/mnemonic_v1_compatibility.json")
	require.NoError(t, err)
	var fixture struct {
		Passphrase string `json:"passphrase"`
		Mnemonic   string `json:"mnemonic"`
		DER        string `json:"der_hex"`
		Encrypted  string `json:"encrypted_hex"`
	}
	require.NoError(t, json.Unmarshal(content, &fixture))
	der, err := hex.DecodeString(fixture.DER)
	require.NoError(t, err)
	data, err := hex.DecodeString(fixture.Encrypted)
	require.NoError(t, err)
	require.Equal(t, fixture.Mnemonic, legacyMnemonicEncode(data))
	phrase, err := BytesToMnemonic(data)
	require.NoError(t, err)
	require.Equal(t, fixture.Mnemonic, phrase)
	key, err := x509.ParsePKCS8PrivateKey(der)
	require.NoError(t, err)
	return key, fixture.Passphrase, data, der
}

// legacyMnemonicEncode encodes data according to the frozen v1 length/checksum
// and 11-bit word layout, independently of the production mnemonic encoder.
func legacyMnemonicEncode(data []byte) string {
	payload := append([]byte{1, byte(len(data) >> 8), byte(len(data))}, data...)
	sum := sha256.Sum256(payload)
	payload = append(payload, sum[:4]...)
	var bits strings.Builder
	for _, b := range payload {
		for bit := 7; bit >= 0; bit-- {
			bits.WriteByte('0' + (b>>uint(bit))&1)
		}
	}
	for bits.Len()%11 != 0 {
		bits.WriteByte('0')
	}
	text := bits.String()
	words := make([]string, len(text)/11)
	for i := range words {
		index := 0
		for _, bit := range text[i*11 : (i+1)*11] {
			index = index*2 + int(bit-'0')
		}
		words[i] = mnemonicWordList[index]
	}
	return strings.Join(words, " ")
}

// legacyMnemonicDecode independently recovers v1 encrypted DER from phrase
// using the historical fixed parameters, without the production KDF or decoder.
func legacyMnemonicDecode(t *testing.T, phrase, passphrase string) []byte {
	t.Helper()
	var data []byte
	var accumulator uint32
	bits := 0
	for _, word := range strings.Fields(phrase) {
		index, ok := mnemonicWordIndex[word]
		require.True(t, ok)
		accumulator = accumulator<<11 | uint32(index)
		bits += 11
		for bits >= 8 {
			bits -= 8
			data = append(data, byte(accumulator>>uint(bits)))
		}
	}
	require.Equal(t, byte(1), data[0])
	length := int(binary.BigEndian.Uint16(data[1:3]))
	require.GreaterOrEqual(t, len(data), 3+length+4)
	sum := sha256.Sum256(data[:3+length])
	require.Equal(t, sum[:4], data[3+length:3+length+4])
	payload := data[3 : 3+length]
	require.Equal(t, byte(0xe1), payload[0])
	key := argon2.IDKey([]byte(passphrase), payload[1:33], 3, 65536, 4, 32)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	der, err := gcm.Open(nil, payload[33:45], payload[45:], nil)
	require.NoError(t, err)
	return der
}

// TestMnemonicSecurity_SharedBudget proves both existing APIs wait behind a
// fully occupied package budget, then finish after release. It is intentionally
// nonparallel. This test also compiles on the vulnerable base for reproduction.
func TestMnemonicSecurity_SharedBudget(t *testing.T) {
	key, passphrase, data, _ := mnemonicCompatibilityFixture(t)
	phrase := legacyMnemonicEncode(data)
	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{"export", func() error { _, err := PrikeyToMnemonic(key, WithMnemonicPassphrase(passphrase)); return err }},
		{"import", func() error { _, err := MnemonicToPrikey(phrase, WithMnemonicPassphrase(passphrase)); return err }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			require.NoError(t, argon2idMemoryBudget.Acquire(context.Background(), argon2idMemoryBudgetKiB))
			held := true
			defer func() {
				if held {
					argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
				}
			}()
			before := argon2idDerivations.Load()
			started := make(chan struct{})
			done := make(chan error, 1)
			go func() { close(started); done <- operation.run() }()
			<-started
			select {
			case err := <-done:
				t.Fatalf("mnemonic %s bypassed occupied Argon2id budget: %v", operation.name, err)
			case <-time.After(50 * time.Millisecond):
			}
			require.Equal(t, before, argon2idDerivations.Load())
			argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
			held = false
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(30 * time.Second):
				t.Fatal("mnemonic call did not resume after budget release")
			}
			require.Equal(t, before+1, argon2idDerivations.Load())
		})
	}
}

// TestMnemonicSecurity_HistoricalCompatibility verifies an immutable legacy
// fixture with new readers and new writers with an independent legacy reader.
func TestMnemonicSecurity_HistoricalCompatibility(t *testing.T) {
	key, passphrase, data, der := mnemonicCompatibilityFixture(t)
	phrase := legacyMnemonicEncode(data)
	require.Equal(t, der, legacyMnemonicDecode(t, phrase, passphrase))
	for _, contextual := range []bool{false, true} {
		var decoded crypto.PrivateKey
		var encoded string
		var err error
		if contextual {
			decoded, err = MnemonicToPrikeyContext(context.Background(), phrase, WithMnemonicPassphrase(passphrase))
		} else {
			decoded, err = MnemonicToPrikey(phrase, WithMnemonicPassphrase(passphrase))
		}
		require.NoError(t, err)
		require.Equal(t, key, decoded)
		if contextual {
			encoded, err = PrikeyToMnemonicContext(context.Background(), key, WithMnemonicPassphrase(passphrase))
		} else {
			encoded, err = PrikeyToMnemonic(key, WithMnemonicPassphrase(passphrase))
		}
		require.NoError(t, err)
		require.Equal(t, der, legacyMnemonicDecode(t, encoded, passphrase))
	}
	_, err := MnemonicToPrikey(phrase, WithMnemonicPassphrase("wrong passphrase"))
	require.Error(t, err)
	data[len(data)-1] ^= 1
	_, err = MnemonicToPrikey(legacyMnemonicEncode(data), WithMnemonicPassphrase(passphrase))
	require.Error(t, err, "valid outer checksum must not bypass GCM authentication")
}
