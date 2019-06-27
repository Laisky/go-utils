package utils_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func TestGenerateToken(t *testing.T) {
	j, err := utils.NewJWT(utils.NewJWTCfg([]byte("4738947328rh3ru23f32hf238f238fh28f")))
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	expect := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.dS5DHPA_5vM-A4VIa8pFvag4EYp9PrRjmDtBth-EFYYJ5rprtUf83WTO8AQ1295AaGi0uES2bLmkQA8lQGI4Wg"

	got, err := j.GenerateToken("laisky", time.Date(2119, 1, 1, 0, 0, 0, 0, time.UTC), map[string]interface{}{
		"k1": "v1",
		"k2": "v2",
		"k3": "v3",
	})
	if err != nil {
		t.Errorf("generate token error %+v", err)
	}
	if got != expect {
		t.Errorf("expect %v, got %v", expect, got)
	}
}

type jwtUser struct {
	secret []byte
}

func (u *jwtUser) GetUID() interface{} {
	return "laisky"
}
func (u *jwtUser) LoadSecretByUID(uid interface{}) ([]byte, error) {
	return u.secret, nil
}

func TestGenerateDivideToken(t *testing.T) {
	j, err := utils.NewDivideJWT(utils.NewDivideJWTCfg())
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	expect := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.dS5DHPA_5vM-A4VIa8pFvag4EYp9PrRjmDtBth-EFYYJ5rprtUf83WTO8AQ1295AaGi0uES2bLmkQA8lQGI4Wg"

	u := &jwtUser{secret: []byte("4738947328rh3ru23f32hf238f238fh28f")}
	got, err := j.GenerateToken(u, time.Date(2119, 1, 1, 0, 0, 0, 0, time.UTC), map[string]interface{}{
		"k1": "v1",
		"k2": "v2",
		"k3": "v3",
	})
	if err != nil {
		t.Errorf("generate token error %+v", err)
	}
	if got != expect {
		t.Errorf("expect %v, got %v", expect, got)
	}
}

func TestValidToken(t *testing.T) {
	j, err := utils.NewJWT(utils.NewJWTCfg([]byte("4738947328rh3ru23f32hf238f238fh28f")))
	if err != nil {
		t.Errorf("got error %+v", err)
	}
	expect := map[string]interface{}{
		"k1":                  "v1",
		"k2":                  "v2",
		"k3":                  "v3",
		utils.JWTUserIDKey:    "laisky",
		utils.JWTExpiresAtKey: time.Date(2119, 1, 1, 0, 0, 0, 0, time.UTC).UTC(),
	}
	t.Logf("exp: %v", expect["exp"])
	// correct token
	token := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.dS5DHPA_5vM-A4VIa8pFvag4EYp9PrRjmDtBth-EFYYJ5rprtUf83WTO8AQ1295AaGi0uES2bLmkQA8lQGI4Wg"

	got, err := j.Validate(token)
	if err != nil {
		t.Fatalf("got error %+v", err)
	}
	t.Logf("got: %+v", got)
	for k, ev := range expect {
		if v, ok := got[k]; !ok {
			t.Fatalf("key %v not exists in got", k)
		} else {
			if v == ev {
				continue
			}
			t.Fatalf("value of key %v not match, expect %v, got %v", k, ev, v)
		}
	}

	// check expires
	token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE0MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.DhNK_cmiPkOUs2gU4X3Ue5utd0wHpyaCimnKSrrr4XQmdzgfKpaYbPzlouDa0KUVqDSmYPYaLAi3v6m1geV48g"
	if got, err = j.Validate(token); err == nil {
		t.Logf("got: %v", got)
		t.Fatal("token should be expired")
	} else if !strings.Contains(err.Error(), "token invalidate") {
		t.Fatalf("expect invalidate error, bug got %+v", err)
	}

	// check without `exp`
	token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1aWQiOiJsYWlza3kifQ.74A-PjmIj9Vqwfp8MWQGfeVkSxDbH0N2pA5Ru_r0au8YKhNsvk4H7BH0sz97-i0sf_0Izq-VhRqLQM2qP6qlWA"
	if got, err = j.Validate(token); err == nil {
		t.Fatalf("token should be error since of lack of `%v`", utils.JWTExpiresAtKey)
	} else if !strings.Contains(err.Error(), "unknown expires format") {
		t.Fatalf("expect unknown expires format error, but got: %+v", err)
	}

	// check without `uid`
	token = "eyJhbGciOiJIUzUxMiJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyJ9.nF1_ySCLWUppYjgBLRMjBRtjfqZkaqaT8p3QaVjHlg7qBIRvXPVArdWsqRAKqpA1nAxwQjYnhVI9tOslK-M04w"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be error since of lack of `uid`")
	} else if !strings.Contains(err.Error(), "must contains `uid`") {
		t.Fatalf("expect invalidate error, bug got %+v", err)
	}

	// check different method
	token = "eyJhbGciOiJIUzM4NCJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.SF7MS3drdHjQ2k1cDyiWspnDx6f0QiBpxT0B3NM0it1eHd01fJ25Zh2n8iH42DFa"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be error since of method incorrect`")
	} else if !strings.Contains(err.Error(), "JWT method not allowd") {
		t.Fatalf("expect method error, bug got %+v", err)
	}
	// invalidate method, but should return complete payload
	t.Logf("got: %+v", got)
	for k, ev := range expect {
		if v, ok := got[k]; !ok {
			t.Fatalf("key %v not exists in got", k)
		} else {
			if v == ev ||
				k == j.JWTExpiresAtKey {
				continue
			}
			t.Fatalf("value of key %v not match, expect %v, got %v", k, ev, v)
		}
	}
}

func TestValidDivideToken(t *testing.T) {
	j, err := utils.NewDivideJWT(utils.NewDivideJWTCfg())
	if err != nil {
		t.Errorf("got error %+v", err)
	}
	expect := map[string]interface{}{
		"k1":                  "v1",
		"k2":                  "v2",
		"k3":                  "v3",
		utils.JWTUserIDKey:    "laisky",
		utils.JWTExpiresAtKey: time.Date(2119, 1, 1, 0, 0, 0, 0, time.UTC).UTC(),
	}
	t.Logf("exp: %v", expect["exp"])
	// correct token
	token := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.dS5DHPA_5vM-A4VIa8pFvag4EYp9PrRjmDtBth-EFYYJ5rprtUf83WTO8AQ1295AaGi0uES2bLmkQA8lQGI4Wg"

	u := &jwtUser{secret: []byte("4738947328rh3ru23f32hf238f238fh28f")}
	got, err := j.Validate(u, token)
	if err != nil {
		t.Fatalf("got error %+v", err)
	}
	t.Logf("got: %+v", got)
	for k, ev := range expect {
		if v, ok := got[k]; !ok {
			t.Fatalf("key %v not exists in got", k)
		} else {
			if v == ev {
				continue
			}
			t.Fatalf("value of key %v not match, expect %v, got %v", k, ev, v)
		}
	}

	// check expires
	token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE0MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.DhNK_cmiPkOUs2gU4X3Ue5utd0wHpyaCimnKSrrr4XQmdzgfKpaYbPzlouDa0KUVqDSmYPYaLAi3v6m1geV48g"
	if got, err = j.Validate(u, token); err == nil {
		t.Logf("got: %v", got)
		t.Fatal("token should be expired")
	} else if !strings.Contains(err.Error(), "token invalidate") {
		t.Fatalf("expect invalidate error, bug got %+v", err)
	}

	// check without `exp`
	token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1aWQiOiJsYWlza3kifQ.74A-PjmIj9Vqwfp8MWQGfeVkSxDbH0N2pA5Ru_r0au8YKhNsvk4H7BH0sz97-i0sf_0Izq-VhRqLQM2qP6qlWA"
	if got, err = j.Validate(u, token); err == nil {
		t.Fatalf("token should be error since of lack of `%v`", utils.JWTExpiresAtKey)
	} else if !strings.Contains(err.Error(), "unknown expires format") {
		t.Fatalf("expect unknown expires format error, but got: %+v", err)
	}

	// check without `uid`
	token = "eyJhbGciOiJIUzUxMiJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyJ9.nF1_ySCLWUppYjgBLRMjBRtjfqZkaqaT8p3QaVjHlg7qBIRvXPVArdWsqRAKqpA1nAxwQjYnhVI9tOslK-M04w"
	if got, err = j.Validate(u, token); err == nil {
		t.Error("token should be error since of lack of `uid`")
	} else if !strings.Contains(err.Error(), "must contains `uid`") {
		t.Fatalf("expect invalidate error, bug got %+v", err)
	}

	// check different method
	token = "eyJhbGciOiJIUzM4NCJ9.eyJleHAiOjQ3MDE5NzQ0MDAsImsxIjoidjEiLCJrMiI6InYyIiwiazMiOiJ2MyIsInVpZCI6ImxhaXNreSJ9.SF7MS3drdHjQ2k1cDyiWspnDx6f0QiBpxT0B3NM0it1eHd01fJ25Zh2n8iH42DFa"
	if got, err = j.Validate(u, token); err == nil {
		t.Error("token should be error since of method incorrect`")
	} else if !strings.Contains(err.Error(), "JWT method not allowd") {
		t.Fatalf("expect method error, bug got %+v", err)
	}
	// invalidate method, but should return complete payload
	t.Logf("got: %+v", got)
	for k, ev := range expect {
		if v, ok := got[k]; !ok {
			t.Fatalf("key %v not exists in got", k)
		} else {
			if v == ev ||
				k == j.JWTExpiresAtKey {
				continue
			}
			t.Fatalf("value of key %v not match, expect %v, got %v", k, ev, v)
		}
	}
}

func TestPassword(t *testing.T) {
	password := []byte("1234567890")
	hp, err := utils.GeneratePasswordHash(password)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	t.Logf("got hashed password: %v", string(hp))

	if !utils.ValidatePasswordHash(hp, password) {
		t.Fatal("should be validate")
	}
	if utils.ValidatePasswordHash(hp, []byte("dj23fij2f32")) {
		t.Fatal("should not be validate")
	}
}

func ExampleJWT() {
	jwt, err := utils.NewJWT(utils.NewJWTCfg([]byte("your secret key")))
	if err != nil {
		utils.Logger.Panic("try to init jwt got error", zap.Error(err))
	}

	// generate jwt token for user
	// GenerateToken(userId string, expiresAt time.Time, payload map[string]interface{}) (tokenStr string, err error)
	token, err := jwt.GenerateToken("laisky", time.Now().Add(7*24*time.Hour), map[string]interface{}{"display_name": "Laisky"})
	if err != nil {
		utils.Logger.Error("try to generate jwt token got error", zap.Error(err))
		return
	}
	fmt.Println("got token:", token)

	// validate token
	payload, err := jwt.Validate(token)
	if err != nil {
		utils.Logger.Error("token invalidate")
		return
	}
	fmt.Printf("got payload from token: %+v\n", payload)
}

func ExampleDivideJWT() {
	jwt, err := utils.NewDivideJWT(utils.NewDivideJWTCfg())
	if err != nil {
		utils.Logger.Panic("try to init jwt got error", zap.Error(err))
	}

	// generate jwt token for user
	// GenerateToken(userId string, expiresAt time.Time, payload map[string]interface{}) (tokenStr string, err error)
	u := &jwtUser{secret: []byte("secret for this user")}
	token, err := jwt.GenerateToken(u, time.Now().Add(7*24*time.Hour), map[string]interface{}{"display_name": "Laisky"})
	if err != nil {
		utils.Logger.Error("try to generate jwt token got error", zap.Error(err))
		return
	}
	fmt.Println("got token:", token)

	// validate token
	payload, err := jwt.Validate(u, token)
	if err != nil {
		utils.Logger.Error("token invalidate")
		// you can get the payload even the token is invalidate
		utils.Logger.Info("got payload", zap.String("payload", fmt.Sprint(payload)))
		return
	}
	fmt.Printf("got payload from token: %+v\n", payload)
}

func ExampleGeneratePasswordHash() {
	// generate hashed password
	rawPassword := []byte("1234567890")
	hashedPassword, err := utils.GeneratePasswordHash(rawPassword)
	if err != nil {
		utils.Logger.Error("try to generate password got error", zap.Error(err))
		return
	}
	fmt.Printf("got new hashed pasword: %v\n", string(hashedPassword))

	// validate passowrd
	if !utils.ValidatePasswordHash(hashedPassword, rawPassword) {
		utils.Logger.Error("password invalidate", zap.Error(err))
		return
	}
}

func BenchmarkGeneratePasswordHash(b *testing.B) {
	pw := []byte("28jijf23f92of92o3jf23fjo2")
	ph, err := utils.GeneratePasswordHash(pw)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	phw, err := utils.GeneratePasswordHash([]byte("j23foj9foj29fj23fj"))
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}

	b.Run("generate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.GeneratePasswordHash(pw)
		}
	})
	b.Run("validate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.ValidatePasswordHash(ph, pw)
		}
	})
	b.Run("invalidate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.ValidatePasswordHash(phw, pw)
		}
	})
}
