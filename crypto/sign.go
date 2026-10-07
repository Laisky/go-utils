package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"io"
	"math/big"

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

var (
	// SignByRSAWithSHA256 sign content by rsa with sha256
	//
	// Deprecated: use SignByRSAPKCS1v15WithSHA256 instead
	SignByRSAWithSHA256 = SignByRSAPKCS1v15WithSHA256
	// VerifyByRSAWithSHA256 verify signature by rsa with sha256
	//
	// Deprecated: use VerifyByRSAPKCS1v15WithSHA256 instead
	VerifyByRSAWithSHA256 = VerifyByRSAPKCS1v15WithSHA256
)

// SignByRSAPKCS1v15WithSHA256 generate signature by rsa private key use sha256
func SignByRSAPKCS1v15WithSHA256(prikey *rsa.PrivateKey, content []byte) ([]byte, error) {
	hashed := sha256.Sum256(content)
	return rsa.SignPKCS1v15(rand.Reader, prikey, crypto.SHA256, hashed[:])
}

// VerifyByRSAPKCS1v15WithSHA256 verify signature by rsa public key use sha256
func VerifyByRSAPKCS1v15WithSHA256(pubKey *rsa.PublicKey, content []byte, sig []byte) error {
	hash := sha256.Sum256(content)
	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sig)
}

// SignByRSAPSSWithSHA256 generate signature by rsa private key use sha256
//
// RSASSA-PSS is not deterministic, so it will return different signature every time.
func SignByRSAPSSWithSHA256(prikey *rsa.PrivateKey, content []byte) ([]byte, error) {
	hashed := sha256.Sum256(content)
	return rsa.SignPSS(rand.Reader, prikey, crypto.SHA256, hashed[:], nil)
}

// VerifyByRSAPSSWithSHA256 verify signature by rsa public key use sha256
func VerifyByRSAPSSWithSHA256(pubKey *rsa.PublicKey, content []byte, sig []byte) error {
	hash := sha256.Sum256(content)
	return rsa.VerifyPSS(pubKey, crypto.SHA256, hash[:], sig, nil)
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

// SignByEd25519WithSHA512 signs the legacy plain-Ed25519-over-SHA512 format.
// Existing signatures remain unchanged; this function is NOT RFC 8032 Ed25519ph.
//
// Deprecated: use SignByEd25519ph for new prehash protocols, or explicitly select
// SignByEd25519LegacySHA512 when maintaining an existing legacy protocol.
func SignByEd25519WithSHA512(prikey ed25519.PrivateKey, reader io.Reader) ([]byte, error) {
	return SignByEd25519LegacySHA512(prikey, reader)
}

// VerifyByEd25519WithSHA512 verifies only the legacy plain-over-SHA512 format.
// It never falls back between the legacy and Ed25519ph protocols.
//
// Deprecated: use VerifyByEd25519ph for new prehash protocols, or explicitly
// select VerifyByEd25519LegacySHA512 for existing legacy signatures.
func VerifyByEd25519WithSHA512(pubKey ed25519.PublicKey, reader io.Reader, sig []byte) error {
	return VerifyByEd25519LegacySHA512(pubKey, reader, sig)
}

// SignByECDSAWithSHA256 generate signature by ecdsa private key use sha256
func SignByECDSAWithSHA256(prikey *ecdsa.PrivateKey, content []byte) (r, s *big.Int, err error) {
	hash := sha256.Sum256(content)
	return ecdsa.Sign(rand.Reader, prikey, hash[:])
}

// VerifyByECDSAWithSHA256 verify signature by ecdsa public key use sha256
func VerifyByECDSAWithSHA256(pubKey *ecdsa.PublicKey, content []byte, r, s *big.Int) bool {
	if !validECDSAVerificationInputs(pubKey, r, s) {
		return false
	}
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

// VerifyByECDSAWithSHA256AndBase64 verifies a bounded NIST-curve signature.
// Malformed encodings and invalid keys/components return false with an error.
func VerifyByECDSAWithSHA256AndBase64(pubKey *ecdsa.PublicKey, content []byte, signature string) (bool, error) {
	r, s, err := DecodeES256SignByBase64(signature)
	if err != nil {
		return false, errors.Wrap(err, "decode signature")
	}
	if !validECDSAVerificationInputs(pubKey, r, s) {
		return false, errors.New("invalid ECDSA key or signature components")
	}
	hash := sha256.Sum256(content)
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
	if reader == nil {
		return false, errors.New("verify ECDSA: nil reader")
	}
	if !validECDSAVerificationInputs(pubKey, r, s) {
		return false, errors.New("invalid ECDSA key or signature components")
	}
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

// DecodeES256SignByHex parses two positive hex components for the supported NIST curves.
// Input is limited to 265 bytes and 521 bits per component before verification.
func DecodeES256SignByHex(sign string) (r, s *big.Int, err error) {
	return decodeBoundedECDSASignature(sign, false)
}

// EncodeES256SignByBase64 format ecdsa signature to stirng
func EncodeES256SignByBase64(r, s *big.Int) string {
	return FormatBig2Base64(r) + ecdsaSignDelimiter + FormatBig2Base64(s)
}

// DecodeES256SignByBase64 parses two positive canonical URL-Base64 components.
// Input is limited to 177 bytes and 521 bits per component before verification.
func DecodeES256SignByBase64(sign string) (r, s *big.Int, err error) {
	return decodeBoundedECDSASignature(sign, true)
}

// HMACSha256 calculate HMAC by sha256
//
// The main difference between HMAC and SHA is that
// HMAC uses a secure key to calculate the hash, while SHA does not.
// this makes HMAC more secure than SHA.
//
// # Args:
//   - key: secure key, no limit on length
//   - data: raw data to calculate HMAC
//
// # Returns:
//   - hmac: HMAC result, 32 bytes
func HMACSha256(key []byte, data io.Reader) ([]byte, error) {
	h := hmac.New(sha256.New, key)
	if _, err := io.Copy(h, data); err != nil {
		return nil, errors.Wrap(err, "write data")
	}

	return h.Sum(nil), nil
}

// VerifyHMACSha256 verifies data against an expected HMAC-SHA256 signature
// using constant-time comparison to prevent timing attacks.
//
// # Args:
//   - key: secure key used to compute the HMAC
//   - data: raw data to verify
//   - expectedMAC: the expected HMAC signature to compare against (32 bytes)
//
// # Returns:
//   - error if the HMAC does not match or computation fails
func VerifyHMACSha256(key []byte, data io.Reader, expectedMAC []byte) error {
	actual, err := HMACSha256(key, data)
	if err != nil {
		return errors.Wrap(err, "compute hmac")
	}

	if !hmac.Equal(actual, expectedMAC) {
		return errors.Errorf("hmac verification failed")
	}

	return nil
}
