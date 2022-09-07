package encrypt

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v2/log"
)

const (
	testhashraw = "dfij3ifj2jjl2jelkjdkwef"
)

func TestHashSHA128String(t *testing.T) {
	val := testhashraw
	got := HashSHA128String(val)
	if got != "57dce855bbee0bef97b63527d473c807a424511d" {
		t.Fatalf("got: %v", got)
	}
}
func ExampleHashSHA128String() {
	val := testhashraw
	got := HashSHA128String(val)
	log.Shared.Info("hash", zap.String("got", got))
}

func TestHashSHA256String(t *testing.T) {
	val := testhashraw
	got := HashSHA256String(val)
	if got != "fef14c65b3d411fee6b2dbcb791a9536cbf637b153bb1de0aae1b41e3834aebf" {
		t.Fatalf("got: %v", got)
	}
}

func ExampleHashSHA256String() {
	val := testhashraw
	got := HashSHA256String(val)
	log.Shared.Info("hash", zap.String("got", got))
}

func TestHashXxhashString(t *testing.T) {
	val := testhashraw
	got := HashXxhashString(val)
	if got != "6466696a3369666a326a6a6c326a656c6b6a646b776566ef46db3751d8e999" {
		t.Fatalf("got: %v", got)
	}
}

func ExampleHashXxhashString() {
	val := testhashraw
	got := HashXxhashString(val)
	log.Shared.Info("hash", zap.String("got", got))
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
		log.Shared.Error("try to generate password got error", zap.Error(err))
		return
	}
	fmt.Printf("got new hashed pasword: %v\n", string(hashedPassword))

	// validate passowrd
	if !ValidatePasswordHash(hashedPassword, rawPassword) {
		log.Shared.Error("password invalidate", zap.Error(err))
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
	t.Logf("pub: %v", string(pubByte))
	if priByte, err = EncodeECDSAPrivateKey(priKey); err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("pri: %v", string(priByte))

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

	// t.Error()
}

func TestRSAKeySerializer(t *testing.T) {
	var (
		err    error
		priKey *rsa.PrivateKey
	)
	if priKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatalf("%+v", err)
	}

	var (
		priByte, pubByte []byte
	)
	if pubByte, err = EncodeRSAPublicKey(&priKey.PublicKey); err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("pub: %v", string(pubByte))
	if priByte, err = EncodeRSAPrivateKey(priKey); err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("pri: %v", string(priByte))

	var (
		priKey2 *rsa.PrivateKey
		// pubKey2 *rsa.PublicKey
	)
	if _, err = DecodeRSAPublicKey(pubByte); err != nil {
		t.Fatalf("%+v", err)
	}
	if priKey2, err = DecodeRSAPrivateKey(priByte); err != nil {
		t.Fatalf("%+v", err)
	}

	hash := sha256.Sum256([]byte("hello, world"))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priKey2, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("%+v", err)
	}

	t.Logf("generate signature: %x", sig)
	if err = rsa.VerifyPKCS1v15(&priKey.PublicKey, crypto.SHA256, hash[:], sig); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// t.Error()
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

func TestRSAVerify(t *testing.T) {
	var (
		err             error
		priKey, priKey2 *rsa.PrivateKey
	)
	if priKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatalf("%+v", err)
	}

	if priKey2, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatalf("%+v", err)
	}

	// case: correct key
	cnt := []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	sig, err := SignByRSAWithSHA256(priKey, cnt)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if err = VerifyByRSAWithSHA256(&priKey.PublicKey, cnt, sig); err != nil {
		t.Fatalf("%+v", err)
	}

	// case: incorrect cnt
	cnt = []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	sig, err = SignByRSAWithSHA256(priKey, cnt)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if err = VerifyByRSAWithSHA256(&priKey.PublicKey, append(cnt, '2'), sig); err == nil {
		t.Fatalf("should not verify")
	}

	// case: incorrect key
	sig, err = SignByRSAWithSHA256(priKey2, cnt)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if err = VerifyByRSAWithSHA256(&priKey.PublicKey, cnt, sig); err == nil {
		t.Fatalf("should not verify")
	}
}

func ExampleSignByECDSAWithSHA256() {
	priKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Shared.Panic("generate key", zap.Error(err))
	}
	priKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Shared.Panic("generate key", zap.Error(err))
	}

	// case: correct key
	cnt := []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err := SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		log.Shared.Panic("sign", zap.Error(err))
	}
	if !VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		log.Shared.Panic("verify failed")
	}

	// generate string
	encoded := EncodeES256SignByBase64(r, s)
	if _, _, err = DecodeES256SignByBase64(encoded); err != nil {
		log.Shared.Panic("encode and decode", zap.Error(err))
	}

	// case: incorrect cnt
	cnt = []byte("fjijf23lijfl23ijrl32jra9pfie9wpfi")
	r, s, err = SignByECDSAWithSHA256(priKey, cnt)
	if err != nil {
		log.Shared.Panic("sign", zap.Error(err))
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, append(cnt, '2'), r, s) {
		log.Shared.Panic("should not verify")
	}

	// case: incorrect key
	r, s, err = SignByECDSAWithSHA256(priKey2, cnt)
	if err != nil {
		log.Shared.Panic("sign", zap.Error(err))
	}
	if VerifyByECDSAWithSHA256(&priKey.PublicKey, cnt, r, s) {
		log.Shared.Panic("should not verify")
	}
}

func TestFormatBig2Hex(t *testing.T) {
	b := new(big.Int)
	b = b.SetInt64(490348974827092350)
	hex := FormatBig2Hex(b)

	t.Logf("%x, %v", b, hex)
	if fmt.Sprintf("%x", b) != hex {
		t.Fatal("not equal")
	}

	// t.Error()
}

func TestFormatBig2Base64(t *testing.T) {
	b := new(big.Int)
	b = b.SetInt64(490348974827092350)
	r := FormatBig2Base64(b)
	t.Log(r)
	if r != "Bs4Ry2yLuX4=" {
		t.Fatal()
	}

	// t.Error()
}

func TestParseHex2Big(t *testing.T) {
	hex := "6ce11cb6c8bb97e"
	b, ok := ParseHex2Big(hex)
	if !ok {
		t.Fatal()
	}

	t.Logf("%x, %v", b, hex)
	if fmt.Sprintf("%x", b) != hex {
		t.Fatal("not equal")
	}
}

func TestParseBase642Big(t *testing.T) {
	raw := "Bs4Ry2yLuX4="
	b, err := ParseBase642Big(raw)
	if err != nil {
		t.Fatal()
	}

	t.Log(b.String())
	if b.Int64() != 490348974827092350 {
		t.Fatal()
	}

	// t.Error()
}

func TestECDSASignFormatAndParseByHex(t *testing.T) {
	a := new(big.Int)
	a = a.SetInt64(490348974827092350)
	b := new(big.Int)
	b = b.SetInt64(9482039480932482)

	encoded := EncodeES256SignByHex(a, b)
	t.Logf("encoded: %v", encoded)

	a2, b2, err := DecodeES256SignByHex(encoded)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if a2.Cmp(a) != 0 || b2.Cmp(b) != 0 {
		t.Fatalf("got %d, %d", a2, b2)
	}
	// t.Error()
}

func TestECDSASignFormatAndParseByBase64(t *testing.T) {
	a := new(big.Int)
	a = a.SetInt64(490348974827092350)
	b := new(big.Int)
	b = b.SetInt64(9482039480932482)

	encoded := EncodeES256SignByBase64(a, b)
	t.Logf("encoded: %v", encoded)

	a2, b2, err := DecodeES256SignByBase64(encoded)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if a2.Cmp(a) != 0 || b2.Cmp(b) != 0 {
		t.Fatalf("got %d, %d", a2, b2)
	}

	// t.Error()
}

func TestNewAesReaderWrapper(t *testing.T) {
	raw := []byte("fjlf2fjjefjwijf93r23f")
	secret := []byte("fjefil2j3i2lfj32fl2defea")
	cipher, err := AesEncrypt(secret, raw)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	reader := bytes.NewReader(cipher)
	readerWraper, err := NewAesReaderWrapper(reader, secret)
	if err != nil {
		t.Fatal(err)
	}

	got, err := ioutil.ReadAll(readerWraper)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, raw) {
		t.Fatalf("got: %s", string(got))
	}
}

// func Test_expandAesSecret(t *testing.T) {
// 	type args struct {
// 		secret []byte
// 	}
// 	tests := []struct {
// 		name string
// 		args args
// 		want int
// 	}{
// 		{"0", args{[]byte("")}, 16},
// 		{"1", args{[]byte("1")}, 16},
// 		{"2", args{[]byte("12")}, 16},
// 		{"3", args{[]byte("14124")}, 16},
// 		{"4", args{[]byte("1535435535")}, 16},
// 		{"5", args{[]byte("   43242341")}, 16},
// 		{"6", args{[]byte("1111111111111111")}, 16},
// 		{"7", args{[]byte("11111111111111111")}, 24},
// 		{"8", args{[]byte("11111111111111111   ")}, 24},
// 		{"9", args{[]byte("11111111111111111   23423 4324   ")}, 32},
// 		{"10", args{[]byte("11111111111111111   23423 4324   111")}, 32},
// 		{"11", args{[]byte("11111111111111111   23423 4324   111414124")}, 32},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if got := expandAesSecret(tt.args.secret); len(got) != tt.want {
// 				t.Errorf("expandAesSecret() = (%d)%v, want %v", len(got), got, tt.want)
// 			}
// 		})
// 	}

// 	// race
// 	var pool errgroup.Group
// 	secret := make([]byte, 5, 10)
// 	for i := 0; i < 17; i++ {
// 		pool.Go(func() error {
// 			expandAesSecret(secret)
// 			return nil
// 		})
// 	}

// 	if err := pool.Wait(); err != nil {
// 		t.Fatalf("%+v", err)
// 	}
// }

func TestEncryptByAes(t *testing.T) {
	type args struct {
		secret []byte
		cnt    string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"", args{[]byte("fjdwudkwfjwiefweffewfewfjelwifew"), "mmm"}, false},
		{"", args{[]byte("fjdwudkwfjwiefweffewfewfjelwifew"), ""}, true},
		{"", args{[]byte("fjdwudkwfjwiefweffewfewfjelwifeww"), "mmm"}, true},
		{"", args{[]byte("fjdwudkwfjwiefweffewfewjelwifew"), "mmm"}, true},
		{"", args{[]byte(""), "mmm"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AesEncrypt(tt.args.secret, []byte(tt.args.cnt))
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("EncryptByAes() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				return
			}

			decrypted, err := AesDecrypt(tt.args.secret, got)
			if err != nil {
				t.Fatalf("decrypt: %+v", err)
			}
			if string(decrypted) != tt.args.cnt {
				t.Fatalf("decrypted not equal to cnt")
			}
		})
	}
}

func TestNewDHKX(t *testing.T) {
	alice, err := NewDHKX()
	require.NoError(t, err)

	bob, err := NewDHKX()
	require.NoError(t, err)

	alicePub := alice.PublicKey()
	bobPub := bob.PublicKey()

	aliceKey, err := alice.GenerateKey(bobPub)
	require.NoError(t, err)

	bobKey, err := bob.GenerateKey(alicePub)
	require.NoError(t, err)

	t.Logf("generate key: %+v", hex.EncodeToString(aliceKey))
	require.Equal(t, aliceKey, bobKey)
}

func ExampleDHKX() {
	alice, _ := NewDHKX()

	bob, _ := NewDHKX()

	alicePub := alice.PublicKey()
	bobPub := bob.PublicKey()

	aliceKey, _ := alice.GenerateKey(bobPub)
	bobKey, _ := bob.GenerateKey(alicePub)
	fmt.Println(reflect.DeepEqual(aliceKey, bobKey))
	// Output: true
}

func TestAESEncryptFilesInDir(t *testing.T) {
	dirName, err := ioutil.TempDir("", "go-utils-test-settings")
	require.NoError(t, err)
	defer os.RemoveAll(dirName)

	cnt := []byte("12345")
	err = ioutil.WriteFile(filepath.Join(dirName, "test1.toml"), cnt, os.ModePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(dirName, "test2.toml"), cnt, os.ModePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(dirName, "test3.toml"), cnt, os.ModePerm)
	require.NoError(t, err)

	secret := []byte("laiskyfwejfewjfewlijffed")
	err = AESEncryptFilesInDir(dirName, secret)
	require.NoError(t, err)

	for _, fname := range []string{"test1.toml.enc", "test2.toml.enc", "test3.toml.enc"} {
		fname = filepath.Join(dirName, fname)
		cipher, err := ioutil.ReadFile(fname)
		require.NoError(t, err)

		got, err := AesDecrypt(secret, cipher)
		require.NoError(t, err)

		require.Equal(t, cnt, got)
	}
}
