package journal_test

import (
	"testing"

	"github.com/Laisky/go-utils/journal"
)

func TestNewInt64Set(t *testing.T) {
	s := journal.NewInt64Set()
	for i := int64(0); i < 10; i++ {
		s.Add(i)
	}

	for i := int64(5); i < 10; i++ {
		s.CheckAndRemove(i)
	}

	if !s.CheckAndRemove(3) {
		t.Fatal("should contains 3")
	}
	if s.CheckAndRemove(7) {
		t.Fatal("should not contains 7")
	}
}

func ExampleInt64Set() {
	s := journal.NewInt64Set()
	s.Add(5)

	s.CheckAndRemove(5) // true
	s.CheckAndRemove(3) // false
}
