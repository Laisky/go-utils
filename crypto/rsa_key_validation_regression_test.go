package crypto

import (
	"crypto/rsa"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRSAHelpersRejectMalformedKeysWithoutPanic verifies that every RSA
// encrypt/decrypt helper returns an error, instead of panicking inside
// rsa.PublicKey.Size, when given a nil key or a key without a modulus.
func TestRSAHelpersRejectMalformedKeysWithoutPanic(t *testing.T) {
	t.Parallel()
	payload := []byte("payload")
	cases := map[string]func() error{
		"pkcs1v15 encrypt zero public key": func() error {
			_, err := RSAEncryptByPKCS1v15(&rsa.PublicKey{}, payload)
			return err
		},
		"pkcs1v15 encrypt even exponent": func() error {
			_, err := RSAEncryptByPKCS1v15(&rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 2047), E: 2}, payload)
			return err
		},
		"pkcs1v15 decrypt nil private key": func() error {
			_, err := RSADecryptByPKCS1v15(nil, payload)
			return err
		},
		"pkcs1v15 decrypt zero private key": func() error {
			_, err := RSADecryptByPKCS1v15(&rsa.PrivateKey{}, payload)
			return err
		},
		"oaep decrypt nil private key": func() error {
			_, err := RSADecryptByOAEP(nil, payload)
			return err
		},
		"oaep decrypt zero private key": func() error {
			_, err := RSADecryptByOAEP(&rsa.PrivateKey{}, payload)
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var err error
			require.NotPanics(t, func() { err = call() })
			require.Error(t, err)
		})
	}
}
