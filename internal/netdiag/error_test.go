package netdiag

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEndpoint verifies the diagnostic allowlist on valid and malformed URLs.
func TestEndpoint(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"https://u:password@example.invalid/token?sig=secret#secret", "https://example.invalid"},
		{"http://u:p@[::1]:8080/secret", "http://[::1]:8080"},
		{"https://secret\n.invalid", "[invalid endpoint]"},
		{"//secret/path", "[invalid endpoint]"},
		{"file:///secret", "[invalid endpoint]"},
		{"https:secret", "[invalid endpoint]"},
	} {
		require.Equal(t, tc.want, Endpoint(tc.raw))
	}
}

// TestErrorClassification preserves errors.Is/As without a printable raw cause chain.
func TestErrorClassification(t *testing.T) {
	cause := &url.Error{Op: "SYNTHETIC_SECRET", URL: "https://user:SYNTHETIC_SECRET@example.invalid/SYNTHETIC_SECRET", Err: context.DeadlineExceeded}
	err := New("send", cause.URL, cause)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var typed *url.Error
	require.ErrorAs(t, err, &typed)
	require.Same(t, cause, typed)
	require.Nil(t, errors.Unwrap(err))
	require.True(t, err.(*Error).Timeout())
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		require.NotContains(t, fmt.Sprintf(format, err), "SYNTHETIC_SECRET")
	}
}
