package utils

import (
	"sync"
	"testing"
)

// GoroutineTest testing.T support goroutine
type GoroutineTest struct {
	mu sync.Mutex
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

// Cleanup add cleanup func
func (t *GoroutineTest) Cleanup(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Cleanup(f)
}

// Error call cancal and exit current goroutine
func (t *GoroutineTest) Error(args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Error(args...)
}

// Errorf call cancal and exit current goroutine
func (t *GoroutineTest) Errorf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Errorf(format, args...)
}

// Fail call cancal and exit current goroutine
func (t *GoroutineTest) Fail() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Fail()
}

// FailNow call cancal and exit current goroutine
func (t *GoroutineTest) FailNow() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancel()
	t.TB.FailNow()
}

// Failed call cancal and exit current goroutine
func (t *GoroutineTest) Failed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.TB.Failed()
}

// Fatal call cancal and exit current goroutine
func (t *GoroutineTest) Fatal(args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Fatal(args...)
}

// Fatalf call cancal and exit current goroutine
func (t *GoroutineTest) Fatalf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Fatalf(format, args...)
}

// Helper call cancal and exit current goroutine
func (t *GoroutineTest) Helper() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Helper()
}

// Log call cancal and exit current goroutine
func (t *GoroutineTest) Log(args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Log(args...)
}

// Logf call cancal and exit current goroutine
func (t *GoroutineTest) Logf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Logf(format, args...)
}

// Name call cancal and exit current goroutine
func (t *GoroutineTest) Name() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.TB.Name()
}

// Setenv call cancal and exit current goroutine
func (t *GoroutineTest) Setenv(key, value string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Setenv(key, value)
}

// Skip call cancal and exit current goroutine
func (t *GoroutineTest) Skip(args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Skip(args...)
}

// SkipNow call cancal and exit current goroutine
func (t *GoroutineTest) SkipNow() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.SkipNow()
}

// Skipf call cancal and exit current goroutine
func (t *GoroutineTest) Skipf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TB.Skipf(format, args...)
}

// Skipped call cancal and exit current goroutine
func (t *GoroutineTest) Skipped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.TB.Skipped()
}

// TempDir call cancal and exit current goroutine
func (t *GoroutineTest) TempDir() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.TB.TempDir()
}
