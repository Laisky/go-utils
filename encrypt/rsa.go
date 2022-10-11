package encrypt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"io"

	"github.com/pkg/errors"
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

// SignByRSAWithSHA256 generate signature by rsa private key use sha256
func SignByRSAWithSHA256(priKey *rsa.PrivateKey, content []byte) ([]byte, error) {
	hashed := sha256.Sum256(content)
	return rsa.SignPKCS1v15(rand.Reader, priKey, crypto.SHA256, hashed[:])
}

// VerifyByRSAWithSHA256 verify signature by rsa public key use sha256
func VerifyByRSAWithSHA256(pubKey *rsa.PublicKey, content []byte, sig []byte) error {
	hash := sha256.Sum256(content)
	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sig)
}

// SignReaderByRSAWithSHA256 generate signature by rsa private key use sha256
func SignReaderByRSAWithSHA256(priKey *rsa.PrivateKey, reader io.Reader) (sig []byte, err error) {
	hasher := sha256.New()
	if _, err = io.Copy(hasher, reader); err != nil {
		return nil, errors.Wrap(err, "read content")
	}

	return rsa.SignPKCS1v15(rand.Reader, priKey, crypto.SHA256, hasher.Sum(nil))
}

// VerifyReaderByRSAWithSHA256 verify signature by rsa public key use sha256
func VerifyReaderByRSAWithSHA256(pubKey *rsa.PublicKey, reader io.Reader, sig []byte) error {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return errors.Wrap(err, "read content")
	}

	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hasher.Sum(nil), sig)
}
