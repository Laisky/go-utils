package utils_test

import (
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
