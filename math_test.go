package utils_test

import (
	"testing"

	"github.com/Laisky/go-utils"
)

func TestRound(t *testing.T) {
	if r := utils.Round(123.555555, .5, 3); r != 123.556 {
		t.Errorf("want 123.556, got %v", r)
	}
}

func ExampleRound() {
	utils.Round(123.555555, .5, 3) // got 123.556
}
