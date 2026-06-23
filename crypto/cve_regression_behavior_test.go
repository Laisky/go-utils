package crypto

// This file contains behavior/regression tests derived from a web survey of
// cryptography CVEs and common design errors. Each test pins a security-critical
// property so that a future refactor cannot silently reintroduce a known class
// of vulnerability. References to the relevant CVE/CWE are noted per test.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

// TestCVE_ECDSA_PsychicSignatureRejected guards against the "psychic signature"
// class (CVE-2022-21449): a verifier that accepts r=0 and/or s=0 will accept a
// forged signature for any message. Go's crypto/ecdsa rejects these, and our
// wrappers must keep rejecting them.
//
// CWE-347: Improper Verification of Cryptographic Signature.
func TestCVE_ECDSA_PsychicSignatureRejected(t *testing.T) {
	t.Parallel()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	msg := []byte("authorize: transfer 1000000 to attacker")

	zero := big.NewInt(0)
	one := big.NewInt(1)

	t.Run("r=0,s=0", func(t *testing.T) {
		t.Parallel()
		require.False(t, VerifyByECDSAWithSHA256(&priv.PublicKey, msg, zero, zero))
	})
	t.Run("r=0,s=1", func(t *testing.T) {
		t.Parallel()
		require.False(t, VerifyByECDSAWithSHA256(&priv.PublicKey, msg, zero, one))
	})
	t.Run("r=1,s=0", func(t *testing.T) {
		t.Parallel()
		require.False(t, VerifyByECDSAWithSHA256(&priv.PublicKey, msg, one, zero))
	})
	t.Run("r=nil,s=nil", func(t *testing.T) {
		t.Parallel()
		// Must not panic; must reject.
		require.False(t, VerifyByECDSAWithSHA256(&priv.PublicKey, msg, nil, nil))
	})
	t.Run("base64 zero signature rejected", func(t *testing.T) {
		t.Parallel()
		// "." decodes to r=0,s=0; verification must return (false, nil) — never panic.
		sig := EncodeES256SignByBase64(zero, zero)
		ok, err := VerifyByECDSAWithSHA256AndBase64(&priv.PublicKey, msg, sig)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

// TestCVE_ECDSA_MalformedBase64NoPanic is a direct regression test for the
// nil-wrap bug that previously made VerifyByECDSAWithSHA256AndBase64 panic
// (nil-pointer dereference, an attacker-triggerable DoS) on a malformed
// signature string. It must now return (false, error) for every malformed input.
//
// CWE-248 / CWE-754: Uncaught Exception / Improper Check for Unusual Conditions.
func TestCVE_ECDSA_MalformedBase64NoPanic(t *testing.T) {
	t.Parallel()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	msg := []byte("hello")

	malformed := []string{
		"",
		"garbage-without-delimiter",
		"only.one.has.too.many",
		"!!!invalid!!!.dGVzdA==",
		"dGVzdA==.!!!invalid!!!",
	}

	for _, sig := range malformed {
		sig := sig
		t.Run("sig="+sig, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				ok, verr := VerifyByECDSAWithSHA256AndBase64(&priv.PublicKey, msg, sig)
				require.False(t, ok)
				require.Error(t, verr)
			})
		})
	}
}

// TestCVE_ECDSA_ForgeryAndCrossKeyRejected pins basic signature soundness:
// a signature is bound to both its message and its key pair.
//
// CWE-347: Improper Verification of Cryptographic Signature.
func TestCVE_ECDSA_ForgeryAndCrossKeyRejected(t *testing.T) {
	t.Parallel()

	keyA, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyB, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	msg := []byte("the original message")
	r, s, err := SignByECDSAWithSHA256(keyA, msg)
	require.NoError(t, err)

	require.True(t, VerifyByECDSAWithSHA256(&keyA.PublicKey, msg, r, s),
		"valid signature must verify with the correct key and message")
	require.False(t, VerifyByECDSAWithSHA256(&keyB.PublicKey, msg, r, s),
		"signature must not verify under a different public key")
	require.False(t, VerifyByECDSAWithSHA256(&keyA.PublicKey, []byte("the tampered message"), r, s),
		"signature must not verify for a tampered message")
}

// TestCVE_AESGCM_AdditionalDataBinding pins that AES-GCM additionalData is
// authenticated: decryption must fail if the AAD differs from what was used at
// encryption time. A common design error is to forget to bind context (AAD),
// or to ignore the verification failure.
//
// CWE-353 / OWASP A02:2021 (A04:2025) Cryptographic Failures.
func TestCVE_AESGCM_AdditionalDataBinding(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{0x42}, 32)
	plaintext := []byte("top secret payload")
	aad := []byte("context: user=alice; tenant=acme")

	ct, err := AEADEncrypt(key, plaintext, aad)
	require.NoError(t, err)

	t.Run("correct AAD decrypts", func(t *testing.T) {
		t.Parallel()
		pt, err := AEADDecrypt(key, ct, aad)
		require.NoError(t, err)
		require.Equal(t, plaintext, pt)
	})
	t.Run("wrong AAD fails", func(t *testing.T) {
		t.Parallel()
		_, err := AEADDecrypt(key, ct, []byte("context: user=mallory; tenant=acme"))
		require.Error(t, err, "tampered associated data must be detected")
	})
	t.Run("missing AAD fails", func(t *testing.T) {
		t.Parallel()
		_, err := AEADDecrypt(key, ct, nil)
		require.Error(t, err, "dropping associated data must be detected")
	})
}

// TestCVE_AESGCM_NonRepeatingCiphertext pins that the safe AEADEncrypt wrapper
// prepends a fresh random IV per call, so encrypting the same plaintext twice
// yields different ciphertexts. This guards against accidentally turning the
// scheme deterministic (which would leak plaintext equality) and against the
// catastrophic GCM nonce-reuse failure mode.
//
// CWE-323: Reusing a Nonce, Key Pair in Encryption.
func TestCVE_AESGCM_NonRepeatingCiphertext(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{0x13}, 32)
	pt := []byte("same plaintext encrypted twice")

	ct1, err := AEADEncrypt(key, pt, nil)
	require.NoError(t, err)
	ct2, err := AEADEncrypt(key, pt, nil)
	require.NoError(t, err)

	require.NotEqual(t, ct1, ct2, "random IV must make ciphertexts differ")
	// IVs (first AesGcmIvLen bytes) must differ.
	require.NotEqual(t, ct1[:AesGcmIvLen], ct2[:AesGcmIvLen], "IV must be unique per encryption")

	// Both still decrypt back to the same plaintext.
	got1, err := AEADDecrypt(key, ct1, nil)
	require.NoError(t, err)
	got2, err := AEADDecrypt(key, ct2, nil)
	require.NoError(t, err)
	require.Equal(t, pt, got1)
	require.Equal(t, pt, got2)
}

// TestCVE_AESGCM_WrongIVRejected pins that AEADDecryptBasic authenticates the
// IV/tag: decrypting with the wrong IV must fail rather than return garbage.
func TestCVE_AESGCM_WrongIVRejected(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{0x7e}, 16)
	iv, err := Salt(AesGcmIvLen)
	require.NoError(t, err)
	plaintext := []byte("authenticated payload")

	cipher, tag, err := AEADEncryptBasic(key, plaintext, iv, nil)
	require.NoError(t, err)

	// Correct IV decrypts.
	pt, err := AEADDecryptBasic(key, cipher, iv, tag, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext, pt)

	// Flip one bit of the IV -> auth must fail.
	badIV := append([]byte(nil), iv...)
	badIV[0] ^= 0x01
	_, err = AEADDecryptBasic(key, cipher, badIV, tag, nil)
	require.Error(t, err, "wrong IV must fail authentication")
}

// TestCVE_RSA_OAEP_Roundtrip_And_SchemeConfusion pins RSA-OAEP behavior and
// guards against padding/scheme-confusion errors: OAEP ciphertext must not be
// decryptable with PKCS#1 v1.5, a tampered ciphertext must fail, and the wrong
// key must fail.
//
// Background: PKCS#1 v1.5 is vulnerable to Bleichenbacher padding-oracle attacks
// (the reason OAEP is preferred); mixing the two schemes is a frequent error.
func TestCVE_RSA_OAEP_Roundtrip_And_SchemeConfusion(t *testing.T) {
	t.Parallel()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	plaintext := []byte("rsa-oaep secret message")

	ct, err := RSAEncryptByOAEP(&priv.PublicKey, plaintext)
	require.NoError(t, err)

	t.Run("roundtrip", func(t *testing.T) {
		t.Parallel()
		pt, err := RSADecryptByOAEP(priv, ct)
		require.NoError(t, err)
		require.Equal(t, plaintext, pt)
	})
	t.Run("wrong key fails", func(t *testing.T) {
		t.Parallel()
		_, err := RSADecryptByOAEP(other, ct)
		require.Error(t, err)
	})
	t.Run("tampered ciphertext fails", func(t *testing.T) {
		t.Parallel()
		bad := append([]byte(nil), ct...)
		bad[len(bad)-1] ^= 0xff
		_, err := RSADecryptByOAEP(priv, bad)
		require.Error(t, err)
	})
	t.Run("OAEP ciphertext not decryptable as PKCS1v15", func(t *testing.T) {
		t.Parallel()
		// Scheme confusion must not silently succeed.
		pt, err := RSADecryptByPKCS1v15(priv, ct)
		require.False(t, err == nil && bytes.Equal(pt, plaintext),
			"OAEP ciphertext must not decrypt to plaintext under PKCS1v15")
	})
}

// TestCVE_RSA_PSS_TamperAndSchemeConfusion pins RSASSA-PSS signature behavior:
// it must be non-deterministic, reject tampered messages and wrong keys, and a
// PSS signature must not verify under PKCS#1 v1.5 (algorithm-confusion guard).
//
// CWE-347: Improper Verification of Cryptographic Signature.
func TestCVE_RSA_PSS_TamperAndSchemeConfusion(t *testing.T) {
	t.Parallel()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	msg := []byte("message to be signed by pss")

	sig1, err := SignByRSAPSSWithSHA256(priv, msg)
	require.NoError(t, err)
	sig2, err := SignByRSAPSSWithSHA256(priv, msg)
	require.NoError(t, err)
	require.NotEqual(t, sig1, sig2, "PSS must be randomized (salted)")

	require.NoError(t, VerifyByRSAPSSWithSHA256(&priv.PublicKey, msg, sig1))
	require.Error(t, VerifyByRSAPSSWithSHA256(&priv.PublicKey, []byte("tampered"), sig1),
		"PSS must reject a tampered message")
	require.Error(t, VerifyByRSAPSSWithSHA256(&other.PublicKey, msg, sig1),
		"PSS must reject a wrong key")

	// Algorithm confusion: a PSS signature must not verify under PKCS1v15 and
	// vice-versa.
	require.Error(t, VerifyByRSAPKCS1v15WithSHA256(&priv.PublicKey, msg, sig1),
		"PSS signature must not verify as PKCS1v15")
	pkcsSig, err := SignByRSAPKCS1v15WithSHA256(priv, msg)
	require.NoError(t, err)
	require.Error(t, VerifyByRSAPSSWithSHA256(&priv.PublicKey, msg, pkcsSig),
		"PKCS1v15 signature must not verify as PSS")
}

// TestCVE_HKDF_RFC5869_KnownAnswer pins HKDF-SHA256 against the canonical
// RFC 5869 Appendix A.1 test vector. A correctness lock-in guards against an
// accidental change of hash function or argument order that would silently
// produce different keys.
func TestCVE_HKDF_RFC5869_KnownAnswer(t *testing.T) {
	t.Parallel()

	ikm := bytes.Repeat([]byte{0x0b}, 22)
	salt := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c,
	}
	info := []byte{0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9}
	wantOKM, err := hex.DecodeString(
		"3cb25f25faacd57a90434f64d0362f2a" +
			"2d2d0a90cf1a5a4c5db02d56ecc4c5bf" +
			"34007208d5b887185865")
	require.NoError(t, err)

	results := [][]byte{make([]byte, 42)}
	require.NoError(t, HKDFWithSHA256(ikm, salt, info, results))
	require.Equal(t, wantOKM, results[0], "HKDF-SHA256 must match RFC 5869 A.1 OKM")
}

// TestCVE_PasswordHash_IterationBounds pins that the serialized-hash parser
// rejects out-of-range iteration counts. An attacker-controlled hash string with
// a huge iteration count would otherwise force a CPU-bound re-hash (DoS), and a
// zero/negative count would weaken verification.
//
// CWE-400: Uncontrolled Resource Consumption.
func TestCVE_PasswordHash_IterationBounds(t *testing.T) {
	t.Parallel()

	// salt and hash hex are arbitrary but well-formed; only hashNum matters here.
	saltHex := hex.EncodeToString([]byte("salt"))
	hashHex := hex.EncodeToString([]byte("hash"))

	t.Run("too many iterations rejected", func(t *testing.T) {
		t.Parallel()
		s := "sha256." + big.NewInt(MaxPasswordHashIteration+1).String() + "." + saltHex + "." + hashHex
		_, err := parseHashedPassword(s)
		require.Error(t, err)
	})
	t.Run("zero iterations rejected", func(t *testing.T) {
		t.Parallel()
		_, err := parseHashedPassword("sha256.0." + saltHex + "." + hashHex)
		require.Error(t, err)
	})
	t.Run("negative iterations rejected", func(t *testing.T) {
		t.Parallel()
		_, err := parseHashedPassword("sha256.-5." + saltHex + "." + hashHex)
		require.Error(t, err)
	})
}

// TestCVE_PasswordHash_ConstantTimeMismatch pins that a single-bit change in the
// stored hash makes the derived hashes differ (the verification path uses a
// constant-time compare). Uses the unexported builder to avoid the deliberate
// multi-second verification delay.
func TestCVE_PasswordHash_ConstantTimeMismatch(t *testing.T) {
	t.Parallel()

	salt := []byte("a-fixed-salt-value")
	pw := []byte("correct horse battery staple")

	h1, err := newHashedPasswordWithMinIteration(salt, pw, gutils.HashTypeSha256, MinPasswordHashIteration, legacyMinPasswordHashIteration)
	require.NoError(t, err)
	h2, err := newHashedPasswordWithMinIteration(salt, []byte("wrong password"), gutils.HashTypeSha256, MinPasswordHashIteration, legacyMinPasswordHashIteration)
	require.NoError(t, err)

	require.NotEqual(t, h1.hashedPassword, h2.hashedPassword,
		"different passwords must yield different hashes")
}

// TestCVE_Mnemonic_PassphraseTamperAndConfusion pins the passphrase-protected
// mnemonic path (Argon2id + AES-256-GCM): the wrong passphrase must fail, an
// encrypted blob must not silently decode without a passphrase, and the
// authenticated ciphertext must reject tampering.
//
// CWE-347 / CWE-323: integrity of the encrypted private-key envelope.
func TestCVE_Mnemonic_PassphraseTamperAndConfusion(t *testing.T) {
	t.Parallel()

	// ECDSA P-256 key keeps the test fast (no RSA keygen).
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	const pass = "correct-passphrase"
	mnemonic, err := PrikeyToMnemonic(priv, WithMnemonicPassphrase(pass))
	require.NoError(t, err)

	t.Run("correct passphrase recovers key", func(t *testing.T) {
		t.Parallel()
		got, err := MnemonicToPrikey(mnemonic, WithMnemonicPassphrase(pass))
		require.NoError(t, err)
		gotEC, ok := got.(*ecdsa.PrivateKey)
		require.True(t, ok)
		require.Equal(t, 0, priv.D.Cmp(gotEC.D), "recovered key must match")
	})
	t.Run("wrong passphrase fails", func(t *testing.T) {
		t.Parallel()
		_, err := MnemonicToPrikey(mnemonic, WithMnemonicPassphrase("wrong-passphrase"))
		require.Error(t, err)
	})
	t.Run("encrypted blob without passphrase fails", func(t *testing.T) {
		t.Parallel()
		_, err := MnemonicToPrikey(mnemonic)
		require.Error(t, err, "must not decode an encrypted mnemonic without a passphrase")
	})
}

// TestCVE_Mnemonic_TruncatedEnvelopeRejected pins that the encrypted-DER
// envelope decoder rejects truncated input rather than mis-parsing it.
func TestCVE_Mnemonic_TruncatedEnvelopeRejected(t *testing.T) {
	t.Parallel()

	_, err := mnemonicDecryptDER([]byte{mnemonicEncryptedMarker, 0x00, 0x01}, "pass")
	require.Error(t, err, "truncated encrypted envelope must be rejected")
}

// TestCVE_RSA_SmallKeyEncryptNoPanic pins that the RSA encryption wrappers
// reject undersized (or nil) public keys with a clean error instead of
// panicking. Previously the chunk-sizing code computed `pubkey.Size()-padding`
// and passed a negative length to make(), which panics ("makeslice: len out of
// range") — an attacker-influenced tiny RSA public key could crash the process.
//
// CWE-248 / CWE-754 / CWE-20: Uncaught Exception / Improper Input Validation.
func TestCVE_RSA_SmallKeyEncryptNoPanic(t *testing.T) {
	t.Parallel()

	// smallPub builds an RSA public key with a chosen modulus byte size,
	// bypassing rsa.GenerateKey's minimum-size guard so we can exercise the
	// wrapper's own input validation.
	smallPub := func(bits int) *rsa.PublicKey {
		n := new(big.Int).Lsh(big.NewInt(1), uint(bits-1))
		n.Add(n, big.NewInt(12345))
		return &rsa.PublicKey{N: n, E: 65537}
	}

	plaintext := []byte("hi")

	t.Run("OAEP tiny key returns error", func(t *testing.T) {
		t.Parallel()
		// 512-bit: Size()=64, OAEP chunk = 64 - 2*32 - 2 = -2.
		require.NotPanics(t, func() {
			_, err := RSAEncryptByOAEP(smallPub(512), plaintext)
			require.Error(t, err, "undersized key must error, not panic")
		})
	})
	t.Run("PKCS1v15 tiny key returns error", func(t *testing.T) {
		t.Parallel()
		// 64-bit: Size()=8, PKCS1v15 chunk = 8 - 11 = -3.
		require.NotPanics(t, func() {
			_, err := RSAEncryptByPKCS1v15(smallPub(64), plaintext)
			require.Error(t, err, "undersized key must error, not panic")
		})
	})
	t.Run("nil key returns error", func(t *testing.T) {
		t.Parallel()
		require.NotPanics(t, func() {
			_, err := RSAEncryptByOAEP(nil, plaintext)
			require.Error(t, err)
			_, err = RSAEncryptByPKCS1v15(nil, plaintext)
			require.Error(t, err)
		})
	})
	t.Run("adequate key still works", func(t *testing.T) {
		t.Parallel()
		// Sanity: the guard must not break legitimate 2048-bit keys.
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		ct, err := RSAEncryptByOAEP(&priv.PublicKey, plaintext)
		require.NoError(t, err)
		pt, err := RSADecryptByOAEP(priv, ct)
		require.NoError(t, err)
		require.Equal(t, plaintext, pt)
	})
}

// TestCVE_Ed25519_MalleabilityAndDomainSeparation pins two properties of the
// Ed25519 signing wrappers:
//
//  1. Signature non-malleability: Go's crypto/ed25519 enforces the canonical
//     bound S < L (order of the base point). A "malleated" signature (R, S+L)
//     satisfies the same verification equation [S]B = R + [h]A but uses a
//     non-canonical scalar; a verifier that forgot the S<L check would accept
//     it, enabling signature-substitution / double-spend style attacks.
//     See "Taming the many EdDSAs" and ZIP-215.
//  2. Domain separation: a signature produced over a SHA-512 prehash must not
//     verify under the SHA-256 prehash wrapper (and vice versa), so the two
//     wrappers cannot be confused.
//
// CWE-347: Improper Verification of Cryptographic Signature.
func TestCVE_Ed25519_MalleabilityAndDomainSeparation(t *testing.T) {
	t.Parallel()

	prikey, err := NewEd25519Prikey()
	require.NoError(t, err)
	pubkey := prikey.Public().(ed25519.PublicKey)

	content := []byte("authorize transfer of 1 BTC")
	sig, err := SignByEd25519WithSHA512(prikey, bytes.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, VerifyByEd25519WithSHA512(pubkey, bytes.NewReader(content), sig))

	t.Run("malleated (S+L) signature rejected", func(t *testing.T) {
		t.Parallel()
		// L = order of the ed25519 base point = 2^252 + 27742317777372353535851937790883648493.
		L, ok := new(big.Int).SetString(
			"7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)
		require.True(t, ok)

		// sig layout: R = sig[0:32], S = sig[32:64] (little-endian scalar).
		sBE := make([]byte, 32)
		for i := 0; i < 32; i++ {
			sBE[i] = sig[63-i]
		}
		sPlusL := new(big.Int).Add(new(big.Int).SetBytes(sBE), L)
		beBytes := sPlusL.Bytes()
		padded := make([]byte, 32)
		copy(padded[32-len(beBytes):], beBytes)

		malSig := make([]byte, 64)
		copy(malSig[:32], sig[:32])
		for i := 0; i < 32; i++ {
			malSig[32+i] = padded[31-i] // back to little-endian
		}

		require.Error(t,
			VerifyByEd25519WithSHA512(pubkey, bytes.NewReader(content), malSig),
			"non-canonical scalar S+L must be rejected (S<L enforced)")
	})

	t.Run("SHA-512 signature does not verify under SHA-256 wrapper", func(t *testing.T) {
		t.Parallel()
		// Different prehash => different signed message => must not verify.
		require.Error(t,
			VerifyReaderByEd25519WithSHA256(pubkey, bytes.NewReader(content), sig),
			"a SHA-512-prehash signature must not verify under the SHA-256 wrapper")
	})
}

// TestCVE_TOTP_RFC6238_KnownAnswer pins TOTP (HMAC-SHA1) against the canonical
// RFC 6238 Appendix B test vectors. HMAC-SHA1 is safe for OTP (its security
// reduces to HMAC's PRF property, not SHA-1 collision resistance), so this is a
// correctness lock-in: it guards against an accidental change of secret
// encoding, digit count, period, or hash that would break interoperability with
// every standard authenticator app.
func TestCVE_TOTP_RFC6238_KnownAnswer(t *testing.T) {
	t.Parallel()

	// RFC 6238 Appendix B uses the ASCII seed "12345678901234567890" with
	// 8-digit codes, a 30-second step, and HMAC-SHA1.
	secret := Base32Secret([]byte("12345678901234567890"))
	totp, err := NewTOTP(OTPArgs{
		Base32Secret: secret,
		Digits:       8,
		PeriodSecs:   30,
		Algorithm:    OTPAlgorithmSHA1,
	})
	require.NoError(t, err)

	vectors := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
	}
	for _, v := range vectors {
		v := v
		require.Equal(t, v.want, totp.KeyAt(time.Unix(v.unix, 0)),
			"RFC 6238 vector at T=%d must match", v.unix)
	}
}
