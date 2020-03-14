package utils

import (
	"testing"
)

func TestRound(t *testing.T) {
	if r := Round(123.555555, .5, 3); r != 123.556 {
		t.Errorf("want 123.556, got %v", r)
	}
}

func ExampleRound() {
	Round(123.555555, .5, 3) // got 123.556
}
