package signature

import (
	"crypto"
	"crypto/rsa"
	"io"

	"github.com/Laisky/errors/v2"
	"github.com/niclabs/tcrsa"

	gutils "github.com/Laisky/go-utils/v6"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

const minRSAPublicKeyBits = 1024

// NewKeyShares generate total keyshares for threshold signature,
// any members exceed threshold can generate legal signature.
//
// threshold must in [(total/2)+1, total]
func NewKeyShares(total, threshold int,
	rsabits gcrypto.RSAPrikeyBits) (
	keyShares tcrsa.KeyShareList,
	keyMeta *tcrsa.KeyMeta,
	err error) {
	switch {
	case threshold < 2:
		return nil, nil, errors.Errorf("threshold should greater than 1")
	case threshold < (total/2+1) || threshold > total:
		return nil, nil, errors.Errorf(
			"threshold should be between the %d and %d, but got %d",
			(total/2 + 1), total, threshold)
	case threshold > 65535 || total > 65535: // Add uint16 bound check
		return nil, nil, errors.Errorf(
			"threshold and total must not exceed 65535 (uint16 max value)")
	case int(rsabits) <= 0 || int(rsabits) > 16384: // Add reasonable RSA bits bound check
		return nil, nil, errors.Errorf(
			"RSA bits must be between 1 and 16384")
	}

	// Safe conversions after bounds checking
	rsaBitsInt := int(rsabits)
	if rsaBitsInt < minRSAPublicKeyBits {
		return nil, nil, errors.Errorf(
			"RSA bits must be at least %d to satisfy crypto/rsa minimum key size", minRSAPublicKeyBits)
	}

	keyBitsForGeneration := rsaBitsInt
	if rsaBitsInt == minRSAPublicKeyBits {
		keyBitsForGeneration++
	}
	thresholdUint16 := uint16(threshold) //nolint:gosec // bounded above by 65535 just above.
	totalUint16 := uint16(total)         //nolint:gosec // G115: integer overflow // already checked

	keyShares, keyMeta, err = tcrsa.NewKey(
		keyBitsForGeneration, thresholdUint16, totalUint16, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new key")
	}
	if keyMeta == nil || keyMeta.PublicKey == nil || keyMeta.PublicKey.N == nil {
		return nil, nil, errors.Errorf("generated threshold RSA key metadata is invalid")
	}
	if bits := keyMeta.PublicKey.N.BitLen(); bits < minRSAPublicKeyBits {
		return nil, nil, errors.Errorf(
			"generated RSA modulus %d bits is smaller than minimum %d bits", bits, minRSAPublicKeyBits)
	}

	return keyShares, keyMeta, nil
}

// SignBySHA256 generate signature by threshold members
func SignBySHA256(content io.Reader,
	keyShares tcrsa.KeyShareList,
	keyMeta *tcrsa.KeyMeta) (signature []byte, err error) {
	switch {
	case content == nil:
		return nil, errors.Errorf("content must not be nil")
	case len(keyShares) == 0:
		return nil, errors.Errorf("keyShares must not be empty")
	case keyMeta == nil || keyMeta.PublicKey == nil:
		return nil, errors.Errorf("keyMeta and its PublicKey must not be nil")
	}

	sig, err := gutils.Hash(gutils.HashTypeSha256, content)
	if err != nil {
		return nil, errors.Wrap(err, "calculate hash of content")
	}

	docPKCS1, err := tcrsa.PrepareDocumentHash(
		keyMeta.PublicKey.Size(), crypto.SHA256, sig)
	if err != nil {
		return nil, errors.Wrap(err, "prepare content hash")
	}

	sigShares := make(tcrsa.SigShareList, len(keyShares))
	for i := 0; i < len(keyShares); i++ {
		sigShares[i], err = keyShares[i].Sign(docPKCS1, crypto.SHA256, keyMeta)
		if err != nil {
			return nil, errors.Wrapf(err, "sign document by keyshares[%d]", i)
		}

		if err := sigShares[i].Verify(docPKCS1, keyMeta); err != nil {
			return nil, errors.Wrapf(err, "verify by keyshares[%d]", i)
		}
	}

	signature, err = sigShares.Join(docPKCS1, keyMeta)
	if err != nil {
		return nil, errors.Wrap(err, "join signature")
	}

	return signature, nil
}

// VerifyBySHA256 verify signature by keyMeta.Pubkey
func VerifyBySHA256(content io.Reader, pubkey *rsa.PublicKey, signature []byte) error {
	switch {
	case content == nil:
		return errors.Errorf("content must not be nil")
	case pubkey == nil:
		return errors.Errorf("pubkey must not be nil")
	case len(signature) == 0:
		return errors.Errorf("signature must not be empty")
	}

	hash, err := gutils.Hash(gutils.HashTypeSha256, content)
	if err != nil {
		return errors.Wrap(err, "calculate hash of content")
	}

	if err := rsa.VerifyPKCS1v15(pubkey, crypto.SHA256, hash, signature); err != nil {
		return errors.Wrap(err, "verify by pubkey")
	}

	return nil
}
