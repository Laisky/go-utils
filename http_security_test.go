package utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestJSON_DoS(t *testing.T) {
	hugeSize := 10 * 1024 * 1024 // 10MB
	hugeBody := strings.Repeat("a", hugeSize)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(hugeBody))
	}))
	defer server.Close()

	var resp any
	err := RequestJSON("GET", server.URL, nil, &resp)
	require.Error(t, err)

	// Now it should be truncated to 8192 + len(" (truncated)")
	limit := 8192
	expectedMaxLen := limit + len(" (truncated)")
	if len(err.Error()) > expectedMaxLen {
		t.Errorf("Expected error message to be at most %d bytes, got %d", expectedMaxLen, len(err.Error()))
	}
	require.Contains(t, err.Error(), "(truncated)")
}
