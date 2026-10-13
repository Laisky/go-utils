package crypto

import (
	"bytes"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
)

// TestTongsuoCompatibilityPrivateKeyPEM preserves the original encrypted SEC1 label and headers.
func TestTongsuoCompatibilityPrivateKeyPEM(t *testing.T) {
	ins := newSecurityTestTongsuo(t)
	const password = "disposable-test-password"
	out, err := ins.NewPrikeyWithPassword(t.Context(), password)
	require.NoError(t, err)
	block, rest := pem.Decode(out)
	require.NotNil(t, block)
	require.Empty(t, bytes.TrimSpace(rest))
	require.Equal(t, "EC PRIVATE KEY", block.Type, "existing SEC1-only parsers must still work")
	require.Equal(t, "4,ENCRYPTED", block.Headers["Proc-Type"])
	require.Contains(t, block.Headers["DEK-Info"], "SM4-CBC,")
	plain, err := ins.runCMDWithEnv(t.Context(), []string{"ec", "-in", tongsuoStdinPath, "-passin", "env:_TONGSUO_TEST_PASSIN"}, out, []string{"_TONGSUO_TEST_PASSIN=" + password})
	require.NoError(t, err)
	decoded, _ := pem.Decode(plain)
	require.NotNil(t, decoded)
	require.Equal(t, "EC PRIVATE KEY", decoded.Type)
	_, err = ins.runCMDWithEnv(t.Context(), []string{"ec", "-in", tongsuoStdinPath, "-passin", "env:_TONGSUO_TEST_PASSIN"}, out, []string{"_TONGSUO_TEST_PASSIN=wrong-password"})
	require.Error(t, err)
}

// TestTongsuoCompatibilityCRLPEM preserves historical PEM output while verifying the signature and fields.
func TestTongsuoCompatibilityCRLPEM(t *testing.T) {
	ins := newSecurityTestTongsuo(t)
	key, der := newTongsuoSM2CAForTest(t, ins, "compatibility-crl-root")
	ca := parseSMCertForTest(t, der)
	now := time.Now().UTC().Truncate(time.Second)
	input := newGoRevocationFixtureForTest(t, ca.RawSubject, ca.SubjectKeyId, big.NewInt(42), nil, now, now.Add(time.Hour))
	out, err := ins.SignX509CRL(t.Context(), input, key)
	require.NoError(t, err)
	block, rest := pem.Decode(out)
	require.NotNil(t, block, "existing PEM callers must not receive DER")
	require.Equal(t, "X509 CRL", block.Type)
	require.Empty(t, bytes.TrimSpace(rest))
	crl, err := smx509.ParseRevocationList(block.Bytes)
	require.NoError(t, err)
	require.NoError(t, crl.CheckSignatureFrom(ca))
	requireCRLFieldsSurviveForTest(t, input, crl)
}
