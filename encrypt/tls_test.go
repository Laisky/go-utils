package encrypt

import (
	"crypto"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTLSPrivatekey(t *testing.T) {
	t.Run("err", func(t *testing.T) {
		_, err := NewRSAPrikey(RSAPrikeyBits(123))
		require.Error(t, err)

		_, err = NewECDSAPrikey(ECDSACurve("123"))
		require.Error(t, err)
	})

	rsa2048, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)
	rsa3072, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)
	es224, err := NewECDSAPrikey(ECDSACurveP224)
	require.NoError(t, err)
	es256, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)
	es384, err := NewECDSAPrikey(ECDSACurveP384)
	require.NoError(t, err)
	es521, err := NewECDSAPrikey(ECDSACurveP521)
	require.NoError(t, err)
	edkey, err := NewEd25519Prikey()
	require.NoError(t, err)

	for _, key := range []crypto.PrivateKey{
		rsa2048,
		rsa3072,
		es224,
		es256,
		es384,
		es521,
		edkey,
	} {
		der, err := Prikey2Der(key)
		require.NoError(t, err)

		pem, err := Prikey2Pem(key)
		require.NoError(t, err)

		der2 := Pem2Der(pem)
		require.Equal(t, pem, PrikeyDer2Pem(der2))
		require.Equal(t, der, der2)
		require.Equal(t, der, Pem2Der(pem))

		key, err = Pem2Prikey(pem)
		require.NoError(t, err)
		der2, err = Prikey2Der(key)
		require.NoError(t, err)
		require.Equal(t, der, der2)

		key, err = Der2Prikey(der)
		require.NoError(t, err)
		der2, err = Prikey2Der(key)
		require.NoError(t, err)
		require.Equal(t, der, der2)

		require.NotNil(t, GetPubkeyFromPrikey(key))

		t.Run("cert", func(t *testing.T) {
			der, err := NewX509Cert(key,
				WithX509CertCommonName("laisky"),
				WithX509CertDNS([]string{"laisky"}),
				WithX509CertIsCA(),
				WithX509CertOrganization([]string{"laisky"}),
				WithX509CertValidFrom(time.Now()),
				WithX509CertValidFor(time.Second),
			)
			require.NoError(t, err)

			cert, err := Der2Cert(der)
			require.NoError(t, err)

			pem := Cert2Pem(cert)
			cert, err = Pem2Cert(pem)
			require.NoError(t, err)
			require.Equal(t, der, Cert2Der(cert))
		})
	}
}
