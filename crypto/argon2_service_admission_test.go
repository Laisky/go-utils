package crypto

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// boundedCryptoHandler illustrates an application-owned, fail-fast ingress
// policy shared by crypto routes. It bounds bodies before invoking work and
// propagates a deadline without changing the library's accepted backup format.
func boundedCryptoHandler(slots chan struct{}, maxBody int64, timeout time.Duration, work func(context.Context, []byte) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "invalid or oversized body", http.StatusRequestEntityTooLarge)
			return
		}
		if err := work(ctx, body); err != nil {
			http.Error(w, "operation failed", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// TestMnemonicSecurity_ServiceAdmission tests the documented service boundary
// policy with a real mnemonic call queued behind the package KDF budget.
func TestMnemonicSecurity_ServiceAdmission(t *testing.T) {
	_, passphrase, data, _ := mnemonicCompatibilityFixture(t)
	phrase := legacyMnemonicEncode(data)
	slots := make(chan struct{}, 1)
	entered := make(chan struct{}, 3)
	handler := boundedCryptoHandler(slots, int64(len(phrase)), 5*time.Second, func(ctx context.Context, body []byte) error {
		entered <- struct{}{}
		_, err := MnemonicToPrikeyContext(ctx, string(body), WithMnemonicPassphrase(passphrase))
		return err
	})
	require.NoError(t, argon2idMemoryBudget.Acquire(context.Background(), argon2idMemoryBudgetKiB))
	held := true
	defer func() {
		if held {
			argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/recover", strings.NewReader(phrase)).WithContext(ctx))
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request not admitted")
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/recover", strings.NewReader(phrase)))
	require.Equal(t, http.StatusServiceUnavailable, second.Code)
	require.Empty(t, entered, "rejected request must not invoke crypto")
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled request did not leave queue")
	}
	require.Equal(t, http.StatusBadRequest, first.Code)
	require.Empty(t, slots, "canceled request must release ingress slot")
	oversized := httptest.NewRecorder()
	handler.ServeHTTP(oversized, httptest.NewRequest(http.MethodPost, "/recover", strings.NewReader(phrase+" ")))
	require.Equal(t, http.StatusRequestEntityTooLarge, oversized.Code)
	require.Empty(t, entered, "oversized request must not invoke crypto")
	argon2idMemoryBudget.Release(argon2idMemoryBudgetKiB)
	held = false
	last := httptest.NewRecorder()
	handler.ServeHTTP(last, httptest.NewRequest(http.MethodPost, "/recover", strings.NewReader(phrase)))
	require.Equal(t, http.StatusNoContent, last.Code)
	require.Empty(t, slots)
}
