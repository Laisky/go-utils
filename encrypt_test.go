package utils_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Laisky/go-utils"
)

func TestGenerateToken(t *testing.T) {
	j := utils.JWT{}
	j.Setup("4738947328rh3ru23f32hf238f238fh28f")
	expect := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMjI4Ni0xMS0yMFQxNzo0Njo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1aWQiOiJsYWlza3kifQ.w5ZD0d0QTnsYjzynhFp5C5aEZ4FlsYJ3Kos7kP8UpGfGfcUWcjXULMbswnR7Zt37-E-B7ffv2uSssTVKzdFlIQ"
	ts, err := time.Parse(time.RFC3339, "2286-11-20T17:46:40Z")
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	got, err := j.GenerateToken("laisky", ts, map[string]interface{}{
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
	j := utils.JWT{}
	j.Setup("4738947328rh3ru23f32hf238f238fh28f")
	expect := map[string]interface{}{
		"k1":         "v1",
		"k2":         "v2",
		"k3":         "v3",
		"uid":        "laisky",
		"expires_at": "2286-11-20T17:46:40Z",
	}
	token := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMjI4Ni0xMS0yMFQxNzo0Njo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1aWQiOiJsYWlza3kifQ.w5ZD0d0QTnsYjzynhFp5C5aEZ4FlsYJ3Kos7kP8UpGfGfcUWcjXULMbswnR7Zt37-E-B7ffv2uSssTVKzdFlIQ"

	got, err := j.Validate(token)
	if err != nil {
		t.Errorf("got error %+v", err)
	}
	for k, ev := range expect {
		if v, ok := got[k]; !ok {
			t.Errorf("key %v not exists in got", k)
		} else if ev != v {
			t.Errorf("value of key %v not match, expect %v, got %v", k, ev, v)
		}
	}

	// check expires
	token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMTI4Ni0xMS0yMFQxNzo0Njo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1aWQiOiJsYWlza3kifQ.4BumoKVHYdx4TbQJRg5zHKfsr3UIKKxdryYjwXBE62RwClm0k_qmFqMuD4hXc-xbzkWgcyN845ulMTGb_8UUAg"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be expired")
	} else if !strings.Contains(err.Error(), "token expired at") {
		t.Errorf("expect expired error, bug got %+v", err)
	}

	// check without `expires_at`
	token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1aWQiOiJsYWlza3kifQ.74A-PjmIj9Vqwfp8MWQGfeVkSxDbH0N2pA5Ru_r0au8YKhNsvk4H7BH0sz97-i0sf_0Izq-VhRqLQM2qP6qlWA"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be error since of lack of `expires_at`")
	} else if !strings.Contains(err.Error(), "token do not contains `expires_at`") {
		t.Errorf("expect expired error, bug got %+v", err)
	}

	// check without `uid`
	token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMTI4Ni0xMS0yMFQxNzo0Njo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMifQ.Skw1XQidknFbI4jKYqI90V5uIghq2gC3rHSO6wiACN-cuctxMR9akRurF2T15FsfHgurK45r32b2sK45vh2EKQ"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be error since of lack of `uid`")
	} else if !strings.Contains(err.Error(), "token do not contains `uid`") {
		t.Errorf("expect expired error, bug got %+v", err)
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
