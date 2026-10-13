package crypto

import (
	"bytes"
	"encoding/pem"
	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

// TestTongsuoCompatibilityPEMFormats exercises original names with historical PEM parsers
// and verifies the parsed CRL signature instead of accepting a changed output encoding.
func TestTongsuoCompatibilityPEMFormats(t *testing.T) {
	ins := newSecurityTestTongsuo(t)
	t.Run("EC_PRIVATE_KEY", func(t *testing.T) {
		encoded, err := ins.NewPrikeyWithPassword(t.Context(), "disposable-test-password")
		require.NoError(t, err)
		block, rest := pem.Decode(encoded)
		require.NotNil(t, block)
		require.Empty(t, bytes.TrimSpace(rest))
		require.Equal(t, "EC PRIVATE KEY", block.Type)
		require.Equal(t, "4,ENCRYPTED", block.Headers["Proc-Type"])
		require.Contains(t, block.Headers["DEK-Info"], "SM4-CBC,")
		decoded, err := ins.runCMDWithEnv(t.Context(), []string{"ec", "-in", tongsuoStdinPath, "-passin", "env:_COMPAT_PASS"}, encoded, []string{"_COMPAT_PASS=disposable-test-password"})
		require.NoError(t, err)
		keyBlock, _ := pem.Decode(decoded)
		require.NotNil(t, keyBlock)
		_, err = smx509.ParseECPrivateKey(keyBlock.Bytes)
		require.NoError(t, err)
	})
	t.Run("X509_CRL", func(t *testing.T) {
		key, cert := newTongsuoSM2CAForTest(t, ins, "compatibility CA")
		issuerTemplate := parseSMCertForTest(t, cert)
		now := time.Now().UTC().Truncate(time.Second)
		input := newGoRevocationFixtureForTest(t, issuerTemplate.RawSubject, issuerTemplate.SubjectKeyId, big.NewInt(3), crlEntriesForTest(now), now, now.Add(time.Hour))
		signed, err := ins.SignX509CRL(t.Context(), input, key)
		require.NoError(t, err)
		block, rest := pem.Decode(signed)
		require.NotNil(t, block, "historical callers parse PEM, not DER")
		require.Equal(t, "X509 CRL", block.Type)
		require.Empty(t, bytes.TrimSpace(rest))
		crl, err := smx509.ParseRevocationList(block.Bytes)
		require.NoError(t, err)
		issuer, err := smx509.ParseCertificate(cert)
		require.NoError(t, err)
		require.NoError(t, crl.CheckSignatureFrom(issuer))
	})
}
