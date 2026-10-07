package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3/group/edwards25519"
	"go.dedis.ch/kyber/v3/sign/schnorr"
	"golang.org/x/crypto/bcrypt"
)

// errOptionFailedForTest is a foreign (standard library) error that the failing test options
// below return, so the tests can check that it comes back wrapped with context.
var errOptionFailedForTest = stderrors.New("option failed for test")

// requireWrappedErr asserts that err wraps cause, so errors.Is still matches it, but is not
// returned bare: the message keeps cause's text and adds context in front of it.
func requireWrappedErr(t *testing.T, err, cause error) {
	t.Helper()

	require.Error(t, err)
	require.ErrorIs(t, err, cause)
	require.NotEqual(t, cause.Error(), err.Error(), "the error must carry context, not be returned bare")
	require.Contains(t, err.Error(), cause.Error())
}

// requireWrappedMsg asserts that err is not nil and that its message strictly extends rawMsg,
// the message the wrapped call produces on its own, with added context.
func requireWrappedMsg(t *testing.T, err error, rawMsg string) {
	t.Helper()

	require.Error(t, err)
	require.NotEqual(t, rawMsg, err.Error(), "the error must carry context, not be returned bare")
	require.Contains(t, err.Error(), rawMsg)
}

// TestErrorWrapping_OptionErrors verifies that every option applier in the package wraps the
// error of a failing option with context instead of returning it bare, while errors.Is still
// finds the original error. Option function types are exported, so their errors may come from
// caller code and count as foreign errors. Regression for the documentation-pass finding on
// dhkxOption.applyOpts, NewDHKX, mnemonicOption.apply, x509V3CertOption.applyOpts,
// WithX509CsrOptions, and signCSROption.applyOpts.
func TestErrorWrapping_OptionErrors(t *testing.T) {
	t.Parallel()

	t.Run("NewDHKX", func(t *testing.T) {
		t.Parallel()
		d, err := NewDHKX(func(*dhkxOption) error { return errOptionFailedForTest })
		require.Nil(t, d)
		requireWrappedErr(t, err, errOptionFailedForTest)
	})

	t.Run("dhkxOption.applyOpts", func(t *testing.T) {
		t.Parallel()
		_, err := new(dhkxOption).applyOpts(func(*dhkxOption) error { return errOptionFailedForTest })
		requireWrappedErr(t, err, errOptionFailedForTest)
	})

	t.Run("mnemonicOption.apply", func(t *testing.T) {
		t.Parallel()
		var o mnemonicOption
		err := o.apply(func(*mnemonicOption) error { return errOptionFailedForTest })
		requireWrappedErr(t, err, errOptionFailedForTest)
	})

	t.Run("x509V3CertOption.applyOpts", func(t *testing.T) {
		t.Parallel()
		_, err := new(x509V3CertOption).applyOpts(func(*x509V3CertOption) error { return errOptionFailedForTest })
		requireWrappedErr(t, err, errOptionFailedForTest)
	})

	t.Run("WithX509CsrOptions", func(t *testing.T) {
		t.Parallel()
		opt := WithX509CsrOptions([]X509CSROption{func(*x509CSROption) error { return errOptionFailedForTest }})
		requireWrappedErr(t, opt(new(x509V3CertOption)), errOptionFailedForTest)
	})

	t.Run("signCSROption.applyOpts", func(t *testing.T) {
		t.Parallel()
		_, err := new(signCSROption).applyOpts(nil, func(*signCSROption) error { return errOptionFailedForTest })
		requireWrappedErr(t, err, errOptionFailedForTest)
	})

	t.Run("compatible options still apply", func(t *testing.T) {
		t.Parallel()
		d, err := NewDHKX()
		require.NoError(t, err)
		require.NotNil(t, d)

		var o mnemonicOption
		require.NoError(t, o.apply(WithMnemonicPassphrase("passphrase")))
		require.Equal(t, "passphrase", o.passphrase)
	})
}

// TestErrorWrapping_ForeignErrors verifies that exported functions which call the standard
// library or third-party packages wrap those errors with context instead of returning them
// bare, and that sentinel errors such as rsa.ErrVerification and bcrypt.ErrPasswordTooLong
// remain matchable with errors.Is. Regression for the documentation-pass error-wrapping sweep.
func TestErrorWrapping_ForeignErrors(t *testing.T) {
	t.Parallel()

	garbageDer := []byte{0x30, 0x03, 0x02, 0x01, 0x01}

	t.Run("ParseBase642Big", func(t *testing.T) {
		t.Parallel()
		_, rawErr := base64.URLEncoding.DecodeString("!!!")
		require.Error(t, rawErr)
		_, err := ParseBase642Big("!!!")
		requireWrappedErr(t, err, rawErr)
	})

	t.Run("x509 parsers", func(t *testing.T) {
		t.Parallel()

		_, rawErr := x509.ParseCertificate(garbageDer)
		_, err := Der2Cert(garbageDer)
		requireWrappedMsg(t, err, rawErr.Error())

		_, rawErr = x509.ParseCertificates(garbageDer)
		_, err = Der2Certs(garbageDer)
		requireWrappedMsg(t, err, rawErr.Error())
		_, err = Pem2Certs(CertDer2Pem(garbageDer))
		requireWrappedMsg(t, err, rawErr.Error())

		_, rawErr = x509.ParseCertificateRequest(garbageDer)
		_, err = Der2CSR(garbageDer)
		requireWrappedMsg(t, err, rawErr.Error())

		_, rawErr = x509.ParseRevocationList(garbageDer)
		_, err = Der2CRL(garbageDer)
		requireWrappedMsg(t, err, rawErr.Error())

		_, rawErr = x509.ParsePKCS1PrivateKey(garbageDer)
		_, err = RSADer2Prikey(garbageDer)
		requireWrappedMsg(t, err, rawErr.Error())
	})

	t.Run("key marshalers", func(t *testing.T) {
		t.Parallel()

		// A NIST curve under a custom name is not a named curve x509 can encode.
		params := *elliptic.P256().Params()
		params.Name = "custom-p256"
		ecPrikey, err := ecdsa.GenerateKey(elliptic.P256(), bytes.NewReader(bytes.Repeat([]byte{7}, 256)))
		require.NoError(t, err)
		customPub := &ecdsa.PublicKey{Curve: &params, X: ecPrikey.X, Y: ecPrikey.Y}

		_, rawErr := x509.MarshalPKIXPublicKey(customPub)
		require.Error(t, rawErr)
		_, err = Pubkey2Der(customPub)
		requireWrappedMsg(t, err, rawErr.Error())

		customPri := &ecdsa.PrivateKey{PublicKey: *customPub, D: ecPrikey.D}
		_, rawErr = x509.MarshalPKCS8PrivateKey(customPri)
		require.Error(t, rawErr)
		_, err = Prikey2Der(customPri)
		requireWrappedMsg(t, err, rawErr.Error())
	})

	t.Run("OidAsn2X509", func(t *testing.T) {
		t.Parallel()
		_, rawErr := x509.OIDFromInts([]uint64{3, 1})
		require.Error(t, rawErr)
		_, err := OidAsn2X509(asn1.ObjectIdentifier{3, 1})
		requireWrappedMsg(t, err, rawErr.Error())
	})

	t.Run("rsa verification keeps rsa.ErrVerification", func(t *testing.T) {
		t.Parallel()
		prikey, err := NewRSAPrikey(RSAPrikeyBits2048)
		require.NoError(t, err)
		badSig := bytes.Repeat([]byte{1}, prikey.Size())

		requireWrappedErr(t, VerifyByRSAPKCS1v15WithSHA256(&prikey.PublicKey, []byte("msg"), badSig),
			rsa.ErrVerification)
		requireWrappedErr(t, VerifyByRSAPSSWithSHA256(&prikey.PublicKey, []byte("msg"), badSig),
			rsa.ErrVerification)
		requireWrappedErr(t, VerifyReaderByRSAWithSHA256(&prikey.PublicKey, bytes.NewReader([]byte("msg")), badSig),
			rsa.ErrVerification)

		sig, err := SignByRSAPKCS1v15WithSHA256(prikey, []byte("msg"))
		require.NoError(t, err)
		require.NoError(t, VerifyByRSAPKCS1v15WithSHA256(&prikey.PublicKey, []byte("msg"), sig))
		sig, err = SignByRSAPSSWithSHA256(prikey, []byte("msg"))
		require.NoError(t, err)
		require.NoError(t, VerifyByRSAPSSWithSHA256(&prikey.PublicKey, []byte("msg"), sig))
		sig, err = SignReaderByRSAWithSHA256(prikey, bytes.NewReader([]byte("msg")))
		require.NoError(t, err)
		require.NoError(t, VerifyReaderByRSAWithSHA256(&prikey.PublicKey, bytes.NewReader([]byte("msg")), sig))
	})

	t.Run("schnorr verification", func(t *testing.T) {
		t.Parallel()
		suite := edwards25519.NewBlakeSHA256Ed25519()
		prikey := suite.Scalar().Pick(suite.RandomStream())
		pubkey := suite.Point().Mul(prikey, nil)

		sig, err := SignBySchnorrSha256(suite, prikey, bytes.NewReader([]byte("msg")))
		require.NoError(t, err)
		require.NoError(t, VerifyBySchnorrSha256(suite, pubkey, bytes.NewReader([]byte("msg")), sig))

		badSig := bytes.Clone(sig)
		badSig[0] ^= 0xff
		digest := sha256Sum(t, []byte("msg"))
		rawErr := schnorr.Verify(suite, pubkey, digest, badSig)
		require.Error(t, rawErr)
		err = VerifyBySchnorrSha256(suite, pubkey, bytes.NewReader([]byte("msg")), badSig)
		requireWrappedMsg(t, err, rawErr.Error())
	})

	t.Run("GeneratePasswordHash keeps bcrypt.ErrPasswordTooLong", func(t *testing.T) {
		t.Parallel()
		_, err := GeneratePasswordHash(bytes.Repeat([]byte("a"), 73))
		requireWrappedErr(t, err, bcrypt.ErrPasswordTooLong)
	})

	t.Run("NewTOTP unsupported algorithm", func(t *testing.T) {
		t.Parallel()
		arg := OTPArgs{Base32Secret: "JBSWY3DPEHPK3PXP", Algorithm: "md5"}
		_, rawErr := arg.Hasher()
		require.Error(t, rawErr)
		_, err := NewTOTP(arg)
		requireWrappedMsg(t, err, rawErr.Error())
	})

	t.Run("unsupported private key types", func(t *testing.T) {
		t.Parallel()
		rawErr := validPrikey("not a key")
		require.Error(t, rawErr)

		_, err := NewX509CSR("not a key")
		requireWrappedMsg(t, err, rawErr.Error())
		_, err = NewX509CertByCSR(&x509.Certificate{}, "not a key", nil)
		requireWrappedMsg(t, err, rawErr.Error())
	})

	t.Run("sm4 argument validation", func(t *testing.T) {
		t.Parallel()
		rawErr := validateSm4Key([]byte("short"))
		require.Error(t, rawErr)
		requireWrappedMsg(t, validateSm4KeyAndIV([]byte("short"), make([]byte, 16)), rawErr.Error())
		requireWrappedMsg(t, validateSm4DecryptArgs([]byte("short"), nil, make([]byte, 16), nil), rawErr.Error())
	})
}

// sha256Sum returns the SHA-256 digest of data as a slice, the digest the Schnorr helpers sign.
func sha256Sum(t *testing.T, data []byte) []byte {
	t.Helper()

	digest := sha256.Sum256(data)
	return digest[:]
}
