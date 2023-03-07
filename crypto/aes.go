package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"os"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"

	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/log"
)

// AesEncrypt encrypt bytes by AES GCM
//
// inspired by https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
//
// The key argument should be the AES key,
// either 16, 24, or 32 bytes to select
// AES-128, AES-192, or AES-256.
func AesEncrypt(secret []byte, cnt []byte) ([]byte, error) {
	if len(cnt) == 0 {
		return nil, errors.Errorf("content is empty")
	}

	// generate a new aes cipher
	c, err := aes.NewCipher(secret)
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

	// creates a new byte array the size of the nonce
	// which must be passed to Seal
	nonce := make([]byte, gcm.NonceSize())
	// populates our nonce with a cryptographically secure
	// random sequence
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.Wrap(err, "load nonce")
	}

	// here we encrypt our text using the Seal function
	// Seal encrypts and authenticates plaintext, authenticates the
	// additional data and appends the result to dst, returning the updated
	// slice. The nonce must be NonceSize() bytes long and unique for all
	// time, for a given key.
	return gcm.Seal(nonce, nonce, cnt, nil), nil
}

// AesDecrypt encrypt bytes by AES GCM
//
// inspired by https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
//
// The key argument should be 16, 24, or 32 bytes
func AesDecrypt(secret []byte, encrypted []byte) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, errors.Errorf("encrypted is empty")
	}

	// generate a new aes cipher
	c, err := aes.NewCipher(secret)
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
	if len(encrypted) < nonceSize {
		return nil, errors.Errorf("encrypted too short")
	}

	nonce, encrypted := encrypted[:nonceSize], encrypted[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
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
