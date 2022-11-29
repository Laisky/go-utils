package utils

import "testing"

// GoroutineTest testing.T support goroutine
type GoroutineTest struct {
	testing.TB
	cancel func()
}

// NewGoroutineTest new test for goroutine
//
// any fail will call cancel()
func NewGoroutineTest(t testing.TB, cancel func()) *GoroutineTest {
	return &GoroutineTest{
		TB:     t,
		cancel: cancel,
	}
}

// FailNow call cancal and exit current goroutine
func (t *GoroutineTest) FailNow() {
	t.cancel()
	t.TB.FailNow()
}
