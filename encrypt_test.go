package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/Laisky/zap"
)

func TestHashSHA128String(t *testing.T) {
	val := "dfij3ifj2jjl2jelkjdkwef"
	got := HashSHA128String(val)
	if got != "6466696a3369666a326a6a6c326a656c6b6a646b776566da39a3ee5e6b4b0d3255bfef95601890afd80709" {
		t.Fatalf("got: %v", got)
	}
}
func ExampleHashSHA128String() {
	val := "dfij3ifj2jjl2jelkjdkwef"
	got := HashSHA128String(val)
	Logger.Info("hash", zap.String("got", got))
}

func TestHashSHA256String(t *testing.T) {
	val := "dfij3ifj2jjl2jelkjdkwef"
	got := HashSHA256String(val)
	if got != "6466696a3369666a326a6a6c326a656c6b6a646b776566e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("got: %v", got)
	}
}

func ExampleHashSHA256String() {
	val := "dfij3ifj2jjl2jelkjdkwef"
	got := HashSHA256String(val)
	Logger.Info("hash", zap.String("got", got))
}

func TestHashXxhashString(t *testing.T) {
	val := "dfij3ifj2jjl2jelkjdkwef"
	got := HashXxhashString(val)
	if got != "6466696a3369666a326a6a6c326a656c6b6a646b776566ef46db3751d8e999" {
		t.Fatalf("got: %v", got)
	}
}

func ExampleHashXxhashString() {
	val := "dfij3ifj2jjl2jelkjdkwef"
	got := HashXxhashString(val)
	Logger.Info("hash", zap.String("got", got))
}

func TestPassword(t *testing.T) {
	password := []byte("1234567890")
	hp, err := GeneratePasswordHash(password)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	t.Logf("got hashed password: %v", string(hp))

	if !ValidatePasswordHash(hp, password) {
		t.Fatal("should be validate")
	}
	if ValidatePasswordHash(hp, []byte("dj23fij2f32")) {
		t.Fatal("should not be validate")
	}
}

func ExampleGeneratePasswordHash() {
	// generate hashed password
	rawPassword := []byte("1234567890")
	hashedPassword, err := GeneratePasswordHash(rawPassword)
	if err != nil {
		Logger.Error("try to generate password got error", zap.Error(err))
		return
	}
	fmt.Printf("got new hashed pasword: %v\n", string(hashedPassword))

	// validate passowrd
	if !ValidatePasswordHash(hashedPassword, rawPassword) {
		Logger.Error("password invalidate", zap.Error(err))
		return
	}
}

func BenchmarkGeneratePasswordHash(b *testing.B) {
	pw := []byte("28jijf23f92of92o3jf23fjo2")
	ph, err := GeneratePasswordHash(pw)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	phw, err := GeneratePasswordHash([]byte("j23foj9foj29fj23fj"))
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}

	b.Run("generate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = GeneratePasswordHash(pw); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})
	b.Run("validate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ValidatePasswordHash(ph, pw)
		}
	})
	b.Run("invalidate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ValidatePasswordHash(phw, pw)
		}
	})
}

func TestECDSAKeySerializer(t *testing.T) {
	var (
		err    error
		priKey *ecdsa.PrivateKey
	)
	if priKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		t.Fatalf("%+v", err)
	}

	var (
		priByte, pubByte []byte
	)
	if pubByte, err = EncodeECDSAPublicKey(&priKey.PublicKey); err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("pub: %+v", pubByte)
	if priByte, err = EncodeECDSAPrivateKey(priKey); err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("pri: %+v", priByte)

	var (
		priKey2 *ecdsa.PrivateKey
		// pubKey2 *ecdsa.PublicKey
	)
	if _, err = DecodeECDSAPublicKey(pubByte); err != nil {
		t.Fatalf("%+v", err)
	}
	if priKey2, err = DecodeECDSAPrivateKey(priByte); err != nil {
		t.Fatalf("%+v", err)
	}

	hash := sha256.Sum256([]byte("hello, world"))
	r, s, err := ecdsa.Sign(rand.Reader, priKey2, hash[:])
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("generate hash: %x %x", r, s)
	if !ecdsa.Verify(&priKey.PublicKey, hash[:], r, s) {
		t.Fatal("verify failed")
	}
}

func TestECDSAVerify(t *testing.T) {
	priKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	priKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	// case: correct key
	cnt := []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err := SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if !VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		t.Fatalf("verify failed")
	}

	// case: incorrect cnt
	cnt = []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err = SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, append(cnt, '2'), r, s) {
		t.Fatalf("should not verify")
	}

	// case: incorrect key
	r, s, err = SignByECDSAWithSHA256(priKey2, cnt)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		t.Fatalf("should not verify")
	}
}

func ExampleSignByECDSAWithSHA256() {
	priKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		Logger.Panic("generate key", zap.Error(err))
	}
	priKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		Logger.Panic("generate key", zap.Error(err))
	}

	// case: correct key
	cnt := []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err := SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		Logger.Panic("sign", zap.Error(err))
	}
	if !VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		Logger.Panic("verify failed")
	}

	// case: incorrect cnt
	cnt = []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err = SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		Logger.Panic("sign", zap.Error(err))
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, append(cnt, '2'), r, s) {
		Logger.Panic("should not verify")
	}

	// case: incorrect key
	r, s, err = SignByECDSAWithSHA256(priKey2, cnt)
	if err != nil {
		Logger.Panic("sign", zap.Error(err))
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		Logger.Panic("should not verify")
	}
}
