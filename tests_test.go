package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MockTB mock testing.TB
type MockTB struct {
	testing.TB
}

// Cleanup mock testing.TB Cleanup
func (t *MockTB) Cleanup(func()) {
	fmt.Println("Cleanup")
}

// Error mock testing.TB Error
func (t *MockTB) Error(args ...any) {
	fmt.Println("Error")
}

// Errorf mock testing.TB Errorf
func (t *MockTB) Errorf(format string, args ...any) {
	fmt.Println("Errorf")
}

// Fail mock testing.TB Fail
func (t *MockTB) Fail() {
	fmt.Println("Fail")
}

// FailNow mock testing.TB FailNow
func (t *MockTB) FailNow() {
	fmt.Println("FailNow")
}

// Failed mock testing.TB Failed
func (t *MockTB) Failed() bool {
	fmt.Println("Failed")
	return false
}

// Fatal mock testing.TB Fatal
func (t *MockTB) Fatal(args ...any) {
	fmt.Println("Fatal")
}

// Fatalf mock testing.TB Fatalf
func (t *MockTB) Fatalf(format string, args ...any) {
	fmt.Println("Fatalf")
}

// Helper mock testing.TB Helper
func (t *MockTB) Helper() {
	fmt.Println("Helper")
}

// Log mock testing.TB Log
func (t *MockTB) Log(args ...any) {
	fmt.Println("Log")
}

// Logf mock testing.TB Logf
func (t *MockTB) Logf(format string, args ...any) {
	fmt.Println("Logf")
}

// Name mock testing.TB Name
func (t *MockTB) Name() string {
	fmt.Println("Name")
	return ""
}

// Setenv mock testing.TB Setenv
func (t *MockTB) Setenv(key, value string) {
	fmt.Println("Setenv")
}

// Skip mock testing.TB Skip
func (t *MockTB) Skip(args ...any) {
	fmt.Println("Skip")
}

// SkipNow mock testing.TB SkipNow
func (t *MockTB) SkipNow() {
	fmt.Println("SkipNow")
}

// Skipf mock testing.TB Skipf
func (t *MockTB) Skipf(format string, args ...any) {
	fmt.Println("Skipf")
}

// Skipped mock testing.TB Skipped
func (t *MockTB) Skipped() bool {
	fmt.Println("Skipped")
	return false
}

// TempDir mock testing.TB TempDir
func (t *MockTB) TempDir() string {
	fmt.Println("TempDir")
	return ""
}

// Private mock testing.TB private
func (t *MockTB) Private() {
	fmt.Println("private")
}

func TestNewGoroutineTest(t *testing.T) {
	testInGoroutine := func(t testing.TB) {
		time.Sleep(time.Second)
		t.FailNow()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ok := t.Run("fail in goroutine", func(t *testing.T) {
		go testInGoroutine(NewGoroutineTest(&MockTB{}, cancel))
		<-ctx.Done()
	})
	require.True(t, ok)
}
