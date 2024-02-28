package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"io"
	"math/big"
	"strings"

	"github.com/Laisky/errors/v2"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/sign/schnorr"
	dediskey "go.dedis.ch/kyber/v3/util/key"
)

// // EncodeRSAPrivateKey encode rsa private key to pem bytes
// func EncodeRSAPrivateKey(privateKey *rsa.PrivateKey) ([]byte, error) {
// 	x509Encoded := x509.MarshalPKCS1PrivateKey(privateKey)
// 	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509Encoded}), nil
// }

// // EncodeRSAPublicKey encode rsa public key to pem bytes
// func EncodeRSAPublicKey(publicKey *rsa.PublicKey) ([]byte, error) {
// 	x509EncodedPub := x509.MarshalPKCS1PublicKey(publicKey)
// 	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509EncodedPub}), nil
// }

// // DecodeRSAPrivateKey decode rsa private key from pem bytes
// func DecodeRSAPrivateKey(pemEncoded []byte) (*rsa.PrivateKey, error) {
// 	block, _ := pem.Decode(pemEncoded)
// 	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "parse rsa private key")
// 	}

// 	return privateKey, nil
// }

// // DecodeRSAPublicKey decode rsa public key from pem bytes
// func DecodeRSAPublicKey(pemEncodedPub []byte) (*rsa.PublicKey, error) {
// 	blockPub, _ := pem.Decode(pemEncodedPub)
// 	pubkey, err := x509.ParsePKCS1PublicKey(blockPub.Bytes)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "parse rsa public key")
// 	}

// 	return pubkey, nil
// }

// SignBySchnorrSha256 sign content by schnorr
func SignBySchnorrSha256(suite dediskey.Suite, prikey kyber.Scalar, reader io.Reader) ([]byte, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return nil, errors.Wrap(err, "read content")
	}

	return schnorr.Sign(suite, prikey, hasher.Sum(nil))
}

// VerifyBySchnorrSha256 verify signature by schnorr
func VerifyBySchnorrSha256(suite dediskey.Suite, pubkey kyber.Point, reader io.Reader, sig []byte) error {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return errors.Wrap(err, "read content")
	}

	return schnorr.Verify(suite, pubkey, hasher.Sum(nil), sig)
}

// SignByRSAWithSHA256 generate signature by rsa private key use sha256
func SignByRSAWithSHA256(prikey *rsa.PrivateKey, content []byte) ([]byte, error) {
	hashed := sha256.Sum256(content)
	return rsa.SignPKCS1v15(rand.Reader, prikey, crypto.SHA256, hashed[:])
}

// VerifyByRSAWithSHA256 verify signature by rsa public key use sha256
func VerifyByRSAWithSHA256(pubKey *rsa.PublicKey, content []byte, sig []byte) error {
	hash := sha256.Sum256(content)
	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sig)
}

// SignReaderByRSAWithSHA256 generate signature by rsa private key use sha256
func SignReaderByRSAWithSHA256(prikey *rsa.PrivateKey, reader io.Reader) (sig []byte, err error) {
	hasher := sha256.New()
	if _, err = io.Copy(hasher, reader); err != nil {
		return nil, errors.Wrap(err, "read content")
	}

	return rsa.SignPKCS1v15(rand.Reader, prikey, crypto.SHA256, hasher.Sum(nil))
}

// VerifyReaderByRSAWithSHA256 verify signature by rsa public key use sha256
func VerifyReaderByRSAWithSHA256(pubKey *rsa.PublicKey, reader io.Reader, sig []byte) error {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return errors.Wrap(err, "read content")
	}

	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hasher.Sum(nil), sig)
}

// SignByEd25519WithSHA512 generate signature by ed25519 private key
func SignByEd25519WithSHA512(prikey ed25519.PrivateKey, reader io.Reader) ([]byte, error) {
	hasher := sha512.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return nil, errors.Wrap(err, "read content")
	}

	return prikey.Sign(rand.Reader, hasher.Sum(nil), crypto.Hash(0))
}

// VerifyByEd25519WithSHA512 verify signature by ed25519 public key
func VerifyByEd25519WithSHA512(pubKey ed25519.PublicKey, reader io.Reader, sig []byte) error {
	hasher := sha512.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return errors.Wrap(err, "read content")
	}

	if !ed25519.Verify(pubKey, hasher.Sum(nil), sig) {
		return errors.New("invalid signature")
	}

	return nil
}

// SignByECDSAWithSHA256 generate signature by ecdsa private key use sha256
func SignByECDSAWithSHA256(prikey *ecdsa.PrivateKey, content []byte) (r, s *big.Int, err error) {
	hash := sha256.Sum256(content)
	return ecdsa.Sign(rand.Reader, prikey, hash[:])
}

// VerifyByECDSAWithSHA256 verify signature by ecdsa public key use sha256
func VerifyByECDSAWithSHA256(pubKey *ecdsa.PublicKey, content []byte, r, s *big.Int) bool {
	hash := sha256.Sum256(content)
	return ecdsa.Verify(pubKey, hash[:], r, s)
}

// SignByECDSAWithSHA256AndBase64 generate signature by ecdsa private key use sha256
func SignByECDSAWithSHA256AndBase64(prikey *ecdsa.PrivateKey, content []byte) (signature string, err error) {
	hash := sha256.Sum256(content)
	r, s, err := ecdsa.Sign(rand.Reader, prikey, hash[:])
	if err != nil {
		return "", errors.Wrap(err, "sign")
	}

	return EncodeES256SignByBase64(r, s), nil
}

// VerifyByECDSAWithSHA256 verify signature by ecdsa public key use sha256
func VerifyByECDSAWithSHA256AndBase64(pubKey *ecdsa.PublicKey, content []byte, signature string) (bool, error) {
	hash := sha256.Sum256(content)
	r, s, err := DecodeES256SignByBase64(signature)
	if err != nil {
		return false, errors.Wrap(err, "decode signature")
	}

	return ecdsa.Verify(pubKey, hash[:], r, s), nil
}

// SignReaderByECDSAWithSHA256 generate signature by ecdsa private key use sha256
func SignReaderByECDSAWithSHA256(prikey *ecdsa.PrivateKey, reader io.Reader) (r, s *big.Int, err error) {
	hasher := sha256.New()
	if _, err = io.Copy(hasher, reader); err != nil {
		return nil, nil, errors.Wrap(err, "read content")
	}

	return ecdsa.Sign(rand.Reader, prikey, hasher.Sum(nil))
}

// VerifyReaderByECDSAWithSHA256 verify signature by ecdsa public key use sha256
func VerifyReaderByECDSAWithSHA256(pubKey *ecdsa.PublicKey, reader io.Reader, r, s *big.Int) (bool, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return false, errors.Wrap(err, "read content")
	}

	return ecdsa.Verify(pubKey, hasher.Sum(nil), r, s), nil
}

const (
	streamChunkSize = 4 * 1024 * 1024
)

// SignReaderByEd25519WithSHA256 generate signature by ecdsa private key use sha256
func SignReaderByEd25519WithSHA256(prikey ed25519.PrivateKey, reader io.Reader) (sig []byte, err error) {
	hasher := sha256.New()
	chunk := make([]byte, streamChunkSize)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		if _, err = hasher.Write(chunk[:n]); err != nil {
			return nil, errors.Wrap(err, "write chunk")
		}
	}

	return prikey.Sign(rand.Reader, hasher.Sum(nil), crypto.Hash(0))
}

// VerifyReaderByEd25519WithSHA256 verify signature by ecdsa public key use sha256
func VerifyReaderByEd25519WithSHA256(pubKey ed25519.PublicKey, reader io.Reader, sig []byte) error {
	hasher := sha256.New()
	chunk := make([]byte, streamChunkSize)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return errors.Wrap(err, "read chunk")
		}

		if _, err = hasher.Write(chunk[:n]); err != nil {
			return errors.Wrap(err, "write chunk")
		}
	}

	err := ed25519.VerifyWithOptions(pubKey, hasher.Sum(nil), sig, &ed25519.Options{
		Hash: crypto.Hash(0),
	})
	return errors.Wrap(err, "verify")
}

const ecdsaSignDelimiter = "."

// EncodeES256SignByHex format ecdsa sign to stirng
func EncodeES256SignByHex(r, s *big.Int) string {
	return FormatBig2Hex(r) + ecdsaSignDelimiter + FormatBig2Hex(s)
}

// DecodeES256SignByHex parse ecdsa signature string to two *big.Int
func DecodeES256SignByHex(sign string) (r, s *big.Int, err error) {
	ss := strings.Split(sign, ecdsaSignDelimiter)
	if len(ss) != 2 {
		return nil, nil, errors.Errorf("unknown format of signature `%s`, want `xxx.xxx`", sign)
	}
	var ok bool
	if r, ok = ParseHex2Big(ss[0]); !ok {
		return nil, nil, errors.Errorf("invalidate hex `%s`", ss[0])
	}
	if s, ok = ParseHex2Big(ss[1]); !ok {
		return nil, nil, errors.Errorf("invalidate hex `%s`", ss[1])
	}

	return
}

// EncodeES256SignByBase64 format ecdsa signature to stirng
func EncodeES256SignByBase64(r, s *big.Int) string {
	return FormatBig2Base64(r) + ecdsaSignDelimiter + FormatBig2Base64(s)
}

// DecodeES256SignByBase64 parse ecdsa signature string to two *big.Int
func DecodeES256SignByBase64(sign string) (r, s *big.Int, err error) {
	ss := strings.Split(sign, ecdsaSignDelimiter)
	if len(ss) != 2 {
		return nil, nil, errors.Wrapf(err, "unknown format of signature `%s`, expect is `xxxx.xxxx`", sign)
	}

	if r, err = ParseBase642Big(ss[0]); err != nil {
		return nil, nil, errors.Wrapf(err, "`%s` is not validate base64", ss[0])
	}

	if s, err = ParseBase642Big(ss[1]); err != nil {
		return nil, nil, errors.Wrapf(err, "`%s` is not validate base64", ss[1])
	}

	return
}
