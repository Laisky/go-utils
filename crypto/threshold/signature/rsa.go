package signature

import (
	"crypto"
	"crypto/rsa"
	"io"

	"github.com/Laisky/errors/v2"
	"github.com/niclabs/tcrsa"

	gutils "github.com/Laisky/go-utils/v4"
	gcrypto "github.com/Laisky/go-utils/v4/crypto"
)

func NewKeyShares(total, threshold int, rsabits gcrypto.RSAPrikeyBits) (keyShares tcrsa.KeyShareList, keyMeta *tcrsa.KeyMeta, err error) {
	switch {
	case threshold < 2:
		return nil, nil, errors.Errorf("threshold should greater than 1")
	case threshold < (total/2+1) || threshold > total:
		return nil, nil, errors.Errorf("threshold should be between the %d and %d, but got %d", (total/2)+1, total, threshold)
	}

	keyShares, keyMeta, err = tcrsa.NewKey(int(rsabits), uint16(threshold), uint16(total), nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new key")
	}

	return keyShares, keyMeta, nil
}

func SignatureBySHA256(content io.Reader, keyShares tcrsa.KeyShareList, keyMeta *tcrsa.KeyMeta) (signature []byte, err error) {
	sig, err := gutils.Hash(gutils.HashTypeSha256, content)
	if err != nil {
		return nil, errors.Wrap(err, "calculate hash of content")
	}

	docPKCS1, err := tcrsa.PrepareDocumentHash(keyMeta.PublicKey.Size(), crypto.SHA256, sig)
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

func VerifyBySHA256(content io.Reader, pubkey *rsa.PublicKey, signature []byte) error {
	hash, err := gutils.Hash(gutils.HashTypeSha256, content)
	if err != nil {
		return errors.Wrap(err, "calculate hash of content")
	}

	if err := rsa.VerifyPKCS1v15(pubkey, crypto.SHA256, hash, signature); err != nil {
		return errors.Wrap(err, "verify by pubkey")
	}

	return nil
}
