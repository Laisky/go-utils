package crypto

import (
	"crypto/aes"
	"crypto/cipher"

	"github.com/Laisky/errors/v2"
)

const (
	// AesGcmIvLen is the length of IV for AES GCM
	AesGcmIvLen = 12
	// AesGcmTagLen is the length of tag for AES GCM
	AesGcmTagLen = 16
	// maxGCMPlaintextLen is the maximum plaintext length AES-GCM can process for
	// a single (key, nonce) pair: (2^32 - 2) blocks of 16 bytes (~64 GiB).
	// crypto/cipher's Seal panics above this; we validate up front and return a
	// clean error instead of letting an oversized input crash the process.
	maxGCMPlaintextLen = ((1 << 32) - 2) * 16
)

// AesEncrypt encrypt bytes by AES GCM
//
// inspired by https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
//
// The key argument should be the AES key,
// either 16, 24, or 32 bytes to select
// AES-128, AES-192, or AES-256.
//
// Deprecated: use AEAD instead
func AesEncrypt(secret []byte, cnt []byte) ([]byte, error) {
	return AEADEncrypt(secret, cnt, nil)
}

// AEADEncrypt encrypt bytes by AES GCM
//
// sugar wrapper of AEADEncryptWithIV, will generate random IV and
// append it to ciphertext as prefix.you can use AEADDecrypt to decrypt it.
//
// # Returns:
//   - ciphertext: consists of IV, cipher and tag, `{iv}{cipher}{tag}`
func AEADEncrypt(key, plaintext, additionalData []byte) (ciphertext []byte, err error) {
	ciphertext = make([]byte, 0, len(plaintext)+AesGcmIvLen+AesGcmTagLen)

	iv, err := Salt(AesGcmIvLen)
	if err != nil {
		return nil, errors.Wrap(err, "generate random iv")
	}

	cipher, tag, err := AEADEncryptBasic(key, plaintext, iv, additionalData)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ciphertext = append(ciphertext, iv...)
	ciphertext = append(ciphertext, cipher...)
	ciphertext = append(ciphertext, tag...)
	return ciphertext, nil
}

// AEADEncryptBasic encrypt bytes by AES GCM and return IV and ciphertext
//
// # Args:
//   - key: AES key, either 16, 24, or 32 bytes to select AES-128, AES-192, or AES-256
//   - plaintext: content to encrypt
//   - iv: Initialization Vector, should be 12 bytes
//   - additionalData: additional data to encrypt
//
// # Returns:
//   - ciphertext: encrypted content without IV and tag, the length of ciphertext is same as plaintext
func AEADEncryptBasic(key, plaintext, iv, additionalData []byte) (ciphertext, tag []byte, err error) {
	if len(plaintext) == 0 {
		return nil, nil, errors.Errorf("content is empty")
	}

	if uint64(len(plaintext)) > maxGCMPlaintextLen {
		return nil, nil, errors.Errorf("plaintext too large for AES-GCM: %d bytes", len(plaintext))
	}

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new aes cipher")
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new gcm")
	}

	if len(iv) != gcm.NonceSize() {
		return nil, nil, errors.Errorf("iv size not match")
	}

	sealed := gcm.Seal(nil, iv, plaintext, additionalData)
	return sealed[:len(plaintext)], sealed[len(plaintext):], nil
}

// AesDecrypt encrypt bytes by AES GCM
//
// inspired by https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
//
// # The key argument should be 16, 24, or 32 bytes
//
// Deprecated: use AEADDecrypt instead
func AesDecrypt(secret []byte, encrypted []byte) ([]byte, error) {
	return AEADDecrypt(secret, encrypted, nil)
}

// AEADDecrypt encrypt bytes by AES GCM
//
// Sugar wrapper of AEADDecryptWithIV, will extract IV from ciphertext automatically.
//
// # Args:
//   - key: AES key, either 16, 24, or 32 bytes to select AES-128, AES-192, or AES-256
//   - ciphertext: encrypted content
//   - additionalData: additional data to encrypt
//
// # Returns:
//   - plaintext: decrypted content
func AEADDecrypt(key, ciphertext, additionalData []byte) (plaintext []byte, err error) {
	if len(ciphertext) == 0 {
		return nil, errors.Errorf("ciphertext is empty")
	}

	// generate a new aes cipher
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "new aes cipher")
	}

	// gcm or Galois/Counter Mode, is a mode of operation
	// for symmetric key cryptographic block ciphers
	// * https://en.wikipedia.org/wiki/Galois/Counter_Mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, errors.Wrap(err, "new gcm")
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err = gcm.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, errors.Wrap(err, "gcm decrypt")
	}

	return plaintext, nil
}

// AEADDecryptBasic encrypt bytes by AES GCM
//
// # Args:
//   - key: AES key, either 16, 24, or 32 bytes to select AES-128, AES-192, or AES-256
//   - ciphertext: encrypted content
//   - iv: Initialization Vector, should be 12 bytes
//   - tag: authentication tag, should be 16 bytes
//   - additionalData: additional data to encrypt
//
// # Returns:
//   - plaintext: decrypted content
func AEADDecryptBasic(key, ciphertext, iv, tag, additionalData []byte) (plaintext []byte, err error) {
	if len(ciphertext) == 0 {
		return nil, errors.Errorf("ciphertext is empty")
	}

	// generate a new aes cipher
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "new aes cipher")
	}

	// gcm or Galois/Counter Mode, is a mode of operation
	// for symmetric key cryptographic block ciphers
	// * https://en.wikipedia.org/wiki/Galois/Counter_Mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, errors.Wrap(err, "new gcm")
	}

	if len(iv) != gcm.NonceSize() {
		return nil, errors.Errorf("iv size not match")
	}

	sealed := make([]byte, 0, len(ciphertext)+len(tag))
	sealed = append(sealed, ciphertext...)
	sealed = append(sealed, tag...)
	plaintext, err = gcm.Open(nil, iv, sealed, additionalData)
	if err != nil {
		return nil, errors.Wrap(err, "gcm decrypt")
	}

	return plaintext, nil
}
