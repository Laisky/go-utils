package utils_test

import (
	"strings"
	"testing"

	"github.com/Laisky/go-utils"
)

func TestGenerateToken(t *testing.T) {
	j := utils.JWT{}
	j.Setup("4738947328rh3ru23f32hf238f238fh28f")
	expect := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMTk3MC0wMS0wMVQwMDowMTo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1c2VybmFtZSI6ImxhaXNreSJ9.5qJK50PRxRpQji5lZmwWdMebMmRugnlw1QxW6kHEPyU"

	got, err := j.Generate(100, map[string]interface{}{
		"k1":       "v1",
		"k2":       "v2",
		"k3":       "v3",
		"username": "laisky",
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
		"username":   "laisky",
		"expires_at": "2286-11-20T17:46:40Z",
	}
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMjI4Ni0xMS0yMFQxNzo0Njo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1c2VybmFtZSI6ImxhaXNreSJ9.pXS75Ske4GdAt8-ZdogD9R5ZcBWRC10BrTg8hxMilXg"

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
	token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMTk3MC0wMS0wMVQwMDowMTo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1c2VybmFtZSI6ImxhaXNreSJ9.5qJK50PRxRpQji5lZmwWdMebMmRugnlw1QxW6kHEPyU"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be expired")
	} else if !strings.Contains(err.Error(), "token expired at") {
		t.Errorf("expect expired error, bug got %+v", err)
	}

	// check without `expires_at`
	token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMiLCJ1c2VybmFtZSI6ImxhaXNreSJ9.FIiw_Cf9B9RqnwfP5KXLdzNCwSOBY0RD-AB9s6bzBvk"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be error since of lack of `expires_at`")
	} else if !strings.Contains(err.Error(), "token do not contains `expires_at`") {
		t.Errorf("expect expired error, bug got %+v", err)
	}

	// check without `username`
	token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHBpcmVzX2F0IjoiMjI4Ni0xMS0yMFQxNzo0Njo0MFoiLCJrMSI6InYxIiwiazIiOiJ2MiIsImszIjoidjMifQ.KlG1cnop21MB5FLtWTnmqwCoCqT8gf087sue3V0Cf3U"
	if got, err = j.Validate(token); err == nil {
		t.Error("token should be error since of lack of `username`")
	} else if !strings.Contains(err.Error(), "token do not contains `username`") {
		t.Errorf("expect expired error, bug got %+v", err)
	}
}
