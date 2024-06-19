package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
	"os"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"

	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/log"
)

const (
	// AesGcmIvLen is the length of IV for AES GCM
	AesGcmIvLen = 12
	// AesGcmTagLen is the length of tag for AES GCM
	AesGcmTagLen = 16
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

	plaintext, err = gcm.Open(nil, iv, append(ciphertext, tag...), additionalData)
	if err != nil {
		return nil, errors.Wrap(err, "gcm decrypt")
	}

	return plaintext, nil
}

// AesReaderWrapper used to decrypt encrypted reader
type AesReaderWrapper struct {
	cnt []byte
	idx int
}

// NewAesReaderWrapper wrap reader by aes
func NewAesReaderWrapper(in io.Reader, key []byte) (*AesReaderWrapper, error) {
	cipher, err := io.ReadAll(in)
	if err != nil {
		return nil, errors.Wrap(err, "read reader")
	}

	w := new(AesReaderWrapper)
	if w.cnt, err = AesDecrypt(key, cipher); err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	return w, nil
}

func (w *AesReaderWrapper) Read(p []byte) (n int, err error) {
	if w.idx == len(w.cnt) {
		return 0, io.EOF
	}

	for n = range p {
		p[n] = w.cnt[w.idx]
		w.idx++
		if w.idx == len(w.cnt) {
			break
		}
	}

	return n + 1, nil
}

const (
	defaultEncryptSuffix = ".enc"
)

type encryptFilesOption struct {
	ext string
	// suffix will append in encrypted file'name after ext as suffix
	suffix string
}

func (o *encryptFilesOption) fillDefault() {
	// o.ext = ".toml"
	o.suffix = defaultEncryptSuffix
}

// AESEncryptFilesInDirOption options to encrypt files in dir
type AESEncryptFilesInDirOption func(*encryptFilesOption) error

// WithAESFilesInDirFileExt only encrypt files with specific ext
func WithAESFilesInDirFileExt(ext string) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if !strings.HasPrefix(ext, ".") {
			return errors.Errorf("ext should start with `.`")
		}

		opt.ext = ext
		return nil
	}
}

// WithAESFilesInDirFileSuffix will append to encrypted's filename as suffix
//
//	xxx.toml -> xxx.toml.enc
func WithAESFilesInDirFileSuffix(suffix string) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if !strings.HasPrefix(suffix, ".") {
			return errors.Errorf("suffix should start with `.`")
		}

		opt.suffix = suffix
		return nil
	}
}

// AESEncryptFilesInDir encrypt files in dir
//
// will generate new encrypted files with <suffix> after ext
//
//	xxx.toml -> xxx.toml.enc
func AESEncryptFilesInDir(dir string, secret []byte, opts ...AESEncryptFilesInDirOption) (err error) {
	opt := new(encryptFilesOption)
	opt.fillDefault()
	for _, optf := range opts {
		if err = optf(opt); err != nil {
			return err
		}
	}
	logger := log.Shared.With(
		zap.String("ext", opt.ext),
		zap.String("suffix", opt.suffix),
	)

	fs, err := gutils.ListFilesInDir(dir)
	if err != nil {
		return errors.Wrapf(err, "read dir `%s`", dir)
	}

	var pool errgroup.Group
	for _, fname := range fs {
		if !strings.HasSuffix(fname, opt.ext) {
			continue
		}

		fname := fname
		pool.Go(func() (err error) {
			raw, err := os.ReadFile(fname)
			if err != nil {
				return errors.Wrapf(err, "read file `%s`", fname)
			}

			cipher, err := AesEncrypt(secret, raw)
			if err != nil {
				return errors.Wrapf(err, "encrypt")
			}

			outfname := fname + opt.suffix
			if err = os.WriteFile(outfname, cipher, 0600); err != nil {
				return errors.Wrapf(err, "write file `%s`", outfname)
			}

			logger.Info("encrypt file", zap.String("src", fname), zap.String("out", outfname))
			return nil
		})
	}

	return pool.Wait()
}
