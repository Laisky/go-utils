package utils_test

import (
	"regexp"
	"testing"

	"github.com/Laisky/go-utils"
)

func TestFallBack(t *testing.T) {
	fail := func() interface{} {
		panic("got error")
	}
	expect := 10
	got := utils.FallBack(fail, 10)
	if expect != got.(int) {
		t.Errorf("expect %v got %v", expect, got)
	}
}

func TestRegexNamedSubMatch(t *testing.T) {
	reg := regexp.MustCompile(`(?P<k1>\w+) (?P<k2>\w+)`)
	str := "abc def"
	submatchMap := map[string]string{}
	if err := utils.RegexNamedSubMatch(reg, str, submatchMap); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	if v1, ok := submatchMap["k1"]; !ok {
		t.Fatalf("k1 should exists")
	} else if v1 != "abc" {
		t.Fatalf("v1 shoule be `abc`, but got: %v", v1)
	}
	if v2, ok := submatchMap["k2"]; !ok {
		t.Fatalf("k2 should exists")
	} else if v2 != "def" {
		t.Fatalf("v2 shoule be `abc`, but got: %v", v2)
	}

}
