package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/alexvec/go-bip39"
	"golang.org/x/crypto/argon2"
)

const (
	// mnemonicVersion is the version byte for the extended mnemonic encoding format.
	mnemonicVersion byte = 0x01

	// mnemonicEncryptedMarker indicates the data is encrypted with a passphrase.
	mnemonicEncryptedMarker byte = 0xE1

	// mnemonicChecksumLen is the number of SHA-256 checksum bytes
	// appended in the extended encoding.
	mnemonicChecksumLen = 4

	// mnemonicArgon2Time is the number of Argon2id iterations.
	mnemonicArgon2Time = 3

	// mnemonicArgon2Memory is the Argon2id memory cost in KiB (64 MiB).
	mnemonicArgon2Memory = 64 * 1024

	// mnemonicArgon2Threads is the Argon2id parallelism factor.
	mnemonicArgon2Threads = 4

	// mnemonicArgon2KeyLen is the derived key length in bytes (AES-256).
	mnemonicArgon2KeyLen = 32

	// mnemonicPassphraseSaltLen is the salt length for passphrase key derivation.
	mnemonicPassphraseSaltLen = 32

	// maxMnemonicDataLen is the maximum data length for BytesToMnemonic.
	maxMnemonicDataLen = 65535

	// maxEncodedMnemonicWords is the maximum number of words that can be
	// produced by the extended mnemonic wire format. Rejecting larger inputs
	// prevents attacker-controlled allocations during decoding.
	maxEncodedMnemonicWords = ((3+maxMnemonicDataLen+mnemonicChecksumLen)*8 + 10) / 11
)

// mnemonicWordList is a snapshot of the BIP39 English word list
// captured at init time to avoid global-state mutations.
var (
	mnemonicWordList  []string
	mnemonicWordIndex map[string]int
)

func init() {
	mnemonicWordList = bip39.GetWordList()
	mnemonicWordIndex = make(map[string]int, len(mnemonicWordList))
	for i, w := range mnemonicWordList {
		mnemonicWordIndex[w] = i
	}
}

// MnemonicOption configures optional behavior for PrikeyToMnemonic / MnemonicToPrikey.
type MnemonicOption func(*mnemonicOption) error

type mnemonicOption struct {
	passphrase string
}

func (o *mnemonicOption) apply(opts ...MnemonicOption) error {
	for _, fn := range opts {
		if err := fn(o); err != nil {
			return err
		}
	}
	return nil
}

// WithMnemonicPassphrase enables passphrase-based encryption for the private key.
// The key is encrypted with AES-256-GCM using a key derived from the passphrase
// via Argon2id (time=3, memory=64MiB, threads=4).
//
// The same passphrase must be provided for both encoding and decoding.
func WithMnemonicPassphrase(passphrase string) MnemonicOption {
	return func(o *mnemonicOption) error {
		if len(passphrase) == 0 {
			return errors.New("passphrase must not be empty")
		}
		o.passphrase = passphrase
		return nil
	}
}

// NewMnemonic generates a random BIP39 mnemonic with the specified bit strength.
//
// Valid bit sizes: 128 (12 words), 160 (15 words), 192 (18 words),
// 224 (21 words), 256 (24 words).
func NewMnemonic(bits int) (string, error) {
	entropy, err := bip39.NewEntropy(bits)
	if err != nil {
		return "", errors.Wrap(err, "generate entropy")
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", errors.Wrap(err, "create mnemonic from entropy")
	}

	return mnemonic, nil
}

// EntropyToMnemonic converts entropy bytes to a standard BIP39 mnemonic phrase.
//
// Entropy must be 16, 20, 24, 28, or 32 bytes (128–256 bits).
func EntropyToMnemonic(entropy []byte) (string, error) {
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", errors.Wrap(err, "entropy to mnemonic")
	}

	return mnemonic, nil
}

// MnemonicToEntropy converts a standard BIP39 mnemonic phrase
// back to the original entropy bytes.
func MnemonicToEntropy(mnemonic string) ([]byte, error) {
	entropy, err := bip39.EntropyFromMnemonic(mnemonic)
	if err != nil {
		return nil, errors.Wrap(err, "mnemonic to entropy")
	}

	return entropy, nil
}

// MnemonicToSeed derives a 512-bit (64-byte) seed from a BIP39 mnemonic
// and optional passphrase using PBKDF2-HMAC-SHA512 with 2048 iterations.
func MnemonicToSeed(mnemonic, passphrase string) ([]byte, error) {
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, passphrase)
	if err != nil {
		return nil, errors.Wrap(err, "mnemonic to seed")
	}

	return seed, nil
}

// ValidateMnemonic checks whether a mnemonic phrase is valid according to BIP39
// (correct word count, all words in word list, valid checksum).
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// BytesToMnemonic encodes arbitrary bytes as a mnemonic phrase
// using the BIP39 English word list.
//
// This uses an extended encoding scheme (NOT standard BIP39) that supports
// arbitrary data lengths up to 65535 bytes. The wire format is:
//
//	version(1) || big-endian-length(2) || data || sha256-checksum(4)
//
// encoded as a stream of 11-bit indices into the BIP39 word list.
//
// Use MnemonicToBytes to decode.
func BytesToMnemonic(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("data must not be empty")
	}
	if len(data) > maxMnemonicDataLen {
		return "", errors.Errorf("data too large: %d bytes, max %d", len(data), maxMnemonicDataLen)
	}

	// Build payload: version || length || data
	payload := make([]byte, 3+len(data))
	payload[0] = mnemonicVersion
	binary.BigEndian.PutUint16(payload[1:3], uint16(len(data)))
	copy(payload[3:], data)

	// Compute and append checksum
	hash := sha256.Sum256(payload)
	full := make([]byte, len(payload)+mnemonicChecksumLen)
	copy(full, payload)
	copy(full[len(payload):], hash[:mnemonicChecksumLen])

	// Convert to 11-bit words
	words := bitsToMnemonicWords(full)
	return strings.Join(words, " "), nil
}

// MnemonicToBytes decodes a mnemonic phrase produced by BytesToMnemonic
// back to the original bytes.
func MnemonicToBytes(mnemonic string) ([]byte, error) {
	words := strings.Fields(mnemonic)
	if len(words) == 0 {
		return nil, errors.New("mnemonic must not be empty")
	}
	if len(words) > maxEncodedMnemonicWords {
		return nil, errors.Errorf("mnemonic too long: %d words, max %d", len(words), maxEncodedMnemonicWords)
	}

	fullBytes, err := mnemonicWordsToBytes(words)
	if err != nil {
		return nil, errors.Wrap(err, "decode mnemonic words")
	}

	// Minimum: version(1) + length(2) + checksum(4) = 7
	if len(fullBytes) < 3+mnemonicChecksumLen {
		return nil, errors.New("mnemonic too short")
	}

	if fullBytes[0] != mnemonicVersion {
		return nil, errors.Errorf("unsupported mnemonic version: 0x%02x", fullBytes[0])
	}

	dataLen := int(binary.BigEndian.Uint16(fullBytes[1:3]))
	requiredLen := 3 + dataLen + mnemonicChecksumLen
	if len(fullBytes) < requiredLen {
		return nil, errors.Errorf("mnemonic data too short: need %d bytes, have %d",
			requiredLen, len(fullBytes))
	}

	payload := fullBytes[:3+dataLen]
	checksum := fullBytes[3+dataLen : requiredLen]

	hash := sha256.Sum256(payload)
	for i := 0; i < mnemonicChecksumLen; i++ {
		if hash[i] != checksum[i] {
			return nil, errors.New("mnemonic checksum verification failed")
		}
	}

	result := make([]byte, dataLen)
	copy(result, payload[3:])
	return result, nil
}

// PrikeyToMnemonic converts a private key (RSA, ECDSA, or Ed25519)
// to a mnemonic phrase.
//
// The key is serialized as PKCS#8 DER and encoded with the extended mnemonic scheme.
// When WithMnemonicPassphrase is provided, the DER bytes are first encrypted
// with AES-256-GCM (key derived via Argon2id) before encoding.
//
// Use MnemonicToPrikey with the same options to recover the key.
func PrikeyToMnemonic(key crypto.PrivateKey, opts ...MnemonicOption) (string, error) {
	var o mnemonicOption
	if err := o.apply(opts...); err != nil {
		return "", errors.Wrap(err, "apply mnemonic options")
	}

	der, err := Prikey2Der(key)
	if err != nil {
		return "", errors.Wrap(err, "serialize private key to DER")
	}

	data := der
	if o.passphrase != "" {
		data, err = mnemonicEncryptDER(der, o.passphrase)
		if err != nil {
			return "", errors.Wrap(err, "encrypt private key")
		}
	}

	return BytesToMnemonic(data)
}

// MnemonicToPrikey recovers a private key from a mnemonic phrase
// produced by PrikeyToMnemonic.
//
// If the key was encrypted with WithMnemonicPassphrase, the same passphrase
// must be provided here.
func MnemonicToPrikey(mnemonic string, opts ...MnemonicOption) (crypto.PrivateKey, error) {
	var o mnemonicOption
	if err := o.apply(opts...); err != nil {
		return nil, errors.Wrap(err, "apply mnemonic options")
	}

	data, err := MnemonicToBytes(mnemonic)
	if err != nil {
		return nil, errors.Wrap(err, "decode mnemonic to bytes")
	}

	der := data
	if len(data) > 0 && data[0] == mnemonicEncryptedMarker {
		if o.passphrase == "" {
			return nil, errors.New("mnemonic data is encrypted but no passphrase provided")
		}
		der, err = mnemonicDecryptDER(data, o.passphrase)
		if err != nil {
			return nil, errors.Wrap(err, "decrypt private key")
		}
	}

	key, err := Der2Prikey(der)
	if err != nil {
		return nil, errors.Wrap(err, "parse private key from DER")
	}

	return key, nil
}

// mnemonicEncryptDER encrypts DER bytes with a passphrase using
// Argon2id key derivation + AES-256-GCM.
//
// Format: encryptedMarker(1) || salt(32) || AES-GCM ciphertext (iv+cipher+tag)
func mnemonicEncryptDER(der []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, mnemonicPassphraseSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, errors.Wrap(err, "generate salt")
	}

	aesKey := argon2.IDKey(
		[]byte(passphrase), salt,
		mnemonicArgon2Time, mnemonicArgon2Memory, mnemonicArgon2Threads,
		mnemonicArgon2KeyLen,
	)

	ciphertext, err := AEADEncrypt(aesKey, der, nil)
	if err != nil {
		return nil, errors.Wrap(err, "AES-GCM encrypt")
	}

	// encryptedMarker || salt || ciphertext
	result := make([]byte, 1+mnemonicPassphraseSaltLen+len(ciphertext))
	result[0] = mnemonicEncryptedMarker
	copy(result[1:], salt)
	copy(result[1+mnemonicPassphraseSaltLen:], ciphertext)
	return result, nil
}

// mnemonicDecryptDER decrypts DER bytes from the encrypted format.
func mnemonicDecryptDER(data []byte, passphrase string) ([]byte, error) {
	minLen := 1 + mnemonicPassphraseSaltLen + AesGcmIvLen + AesGcmTagLen
	if len(data) < minLen {
		return nil, errors.Errorf("encrypted data too short: %d bytes, min %d", len(data), minLen)
	}

	if data[0] != mnemonicEncryptedMarker {
		return nil, errors.Errorf("invalid encrypted marker: 0x%02x", data[0])
	}

	salt := data[1 : 1+mnemonicPassphraseSaltLen]
	ciphertext := data[1+mnemonicPassphraseSaltLen:]

	aesKey := argon2.IDKey(
		[]byte(passphrase), salt,
		mnemonicArgon2Time, mnemonicArgon2Memory, mnemonicArgon2Threads,
		mnemonicArgon2KeyLen,
	)

	der, err := AEADDecrypt(aesKey, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "AES-GCM decrypt (wrong passphrase?)")
	}

	return der, nil
}

// bitsToMnemonicWords converts a byte slice into BIP39 words
// using 11-bit encoding.
func bitsToMnemonicWords(data []byte) []string {
	totalBits := len(data) * 8
	wordCount := (totalBits + 10) / 11 // ceil(totalBits / 11)

	words := make([]string, wordCount)
	for i := range wordCount {
		idx := extract11Bits(data, i*11)
		words[i] = mnemonicWordList[idx]
	}

	return words
}

// extract11Bits extracts an 11-bit big-endian value starting
// at the given bit position in data.
func extract11Bits(data []byte, bitPos int) uint16 {
	var val uint16
	for b := range 11 {
		byteIdx := (bitPos + b) / 8
		bitIdx := 7 - (bitPos+b)%8
		if byteIdx < len(data) && (data[byteIdx]>>uint(bitIdx))&1 == 1 {
			val |= 1 << uint(10-b)
		}
	}
	return val
}

// mnemonicWordsToBytes converts BIP39 words back into a byte slice.
func mnemonicWordsToBytes(words []string) ([]byte, error) {
	totalBits := len(words) * 11
	totalBytes := totalBits / 8

	result := make([]byte, totalBytes)
	for i, word := range words {
		idx, ok := mnemonicWordIndex[word]
		if !ok {
			return nil, errors.Errorf("word %q not found in BIP39 word list", word)
		}
		write11Bits(result, i*11, uint16(idx))
	}

	return result, nil
}

// write11Bits writes an 11-bit big-endian value at the given bit position.
func write11Bits(data []byte, bitPos int, val uint16) {
	for b := range 11 {
		if val&(1<<uint(10-b)) != 0 {
			byteIdx := (bitPos + b) / 8
			bitIdx := 7 - (bitPos+b)%8
			if byteIdx < len(data) {
				data[byteIdx] |= 1 << uint(bitIdx)
			}
		}
	}
}
