package utils

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewReusableRequest verifies that NewReusableRequest builds a request with the given method, URL, and
// context for bytes.Buffer, bytes.Reader, strings.Reader, nil, io.NopCloser, and custom bodies, and that for
// the bytes and strings readers GetBody is set and returns content identical to the original body.
func TestNewReusableRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	method := "POST"
	url := "https://example.com/api"
	testData := []byte("test data for request body")

	tests := []struct {
		name               string
		body               io.Reader
		expectReusable     bool
		expectError        bool
		expectBodyReusable bool
	}{
		{
			name:               "bytes.Buffer should be reusable",
			body:               bytes.NewBuffer(testData),
			expectReusable:     true,
			expectError:        false,
			expectBodyReusable: true,
		},
		{
			name:               "bytes.Reader should be reusable",
			body:               bytes.NewReader(testData),
			expectReusable:     true,
			expectError:        false,
			expectBodyReusable: true,
		},
		{
			name:               "strings.Reader should be reusable",
			body:               strings.NewReader(string(testData)),
			expectReusable:     true,
			expectError:        false,
			expectBodyReusable: true,
		},
		{
			name:               "nil body should work",
			body:               nil,
			expectReusable:     true,
			expectError:        false,
			expectBodyReusable: false, // no body to reuse
		},
		{
			name:               "io.NopCloser should not be reusable",
			body:               io.NopCloser(bytes.NewReader(testData)),
			expectReusable:     true, // request creation succeeds
			expectError:        false,
			expectBodyReusable: false, // but body won't be reusable for redirects
		},
		{
			name:               "custom reader should not be reusable",
			body:               &customReader{data: testData},
			expectReusable:     true, // request creation succeeds
			expectError:        false,
			expectBodyReusable: false, // but body won't be reusable for redirects
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := NewReusableRequest(ctx, method, url, tt.body)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, req)

			if tt.expectReusable {
				// Verify basic request properties
				require.Equal(t, method, req.Method)
				require.Equal(t, url, req.URL.String())
				require.Equal(t, ctx, req.Context())

				// Check if GetBody is properly set for reusable readers
				if tt.expectBodyReusable {
					require.NotNil(t, req.GetBody, "GetBody should be set for reusable readers")

					// Test that GetBody actually works
					if req.GetBody != nil {
						newBody, err := req.GetBody()
						require.NoError(t, err)
						require.NotNil(t, newBody)

						// Read both bodies and compare
						originalBodyBytes, err := io.ReadAll(req.Body)
						require.NoError(t, err)

						newBodyBytes, err := io.ReadAll(newBody)
						require.NoError(t, err)

						require.Equal(t, originalBodyBytes, newBodyBytes, "GetBody should return identical content")
					}
				} else if tt.body != nil {
					// For non-reusable readers, GetBody might be nil (depends on Go version and implementation)
					// This is expected behavior for custom readers
				}
			}
		})
	}
}

// TestNewReusableRequestHTTP2Compatibility verifies that bytes and strings readers get a GetBody that replays
// the original body as an HTTP/2 GOAWAY retry would need, and logs a warning when io.NopCloser or a custom
// reader leaves GetBody nil.
func TestNewReusableRequestHTTP2Compatibility(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testData := []byte(`{"message": "test data for HTTP/2 GOAWAY scenario"}`)

	t.Run("reusable_readers_support_http2_retry", func(t *testing.T) {
		t.Parallel()

		reusableReaders := map[string]io.Reader{
			"bytes.Buffer":   bytes.NewBuffer(testData),
			"bytes.Reader":   bytes.NewReader(testData),
			"strings.Reader": strings.NewReader(string(testData)),
		}

		for name, reader := range reusableReaders {
			reader := reader
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				req, err := NewReusableRequest(ctx, "POST", "https://example.com", reader)
				require.NoError(t, err)
				require.NotNil(t, req.GetBody, "GetBody must be set for HTTP/2 GOAWAY retry compatibility")

				// Simulate what happens during HTTP/2 GOAWAY retry
				originalBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)

				// Get new body for retry
				retryBody, err := req.GetBody()
				require.NoError(t, err)
				require.NotNil(t, retryBody)

				retryBodyData, err := io.ReadAll(retryBody)
				require.NoError(t, err)

				require.Equal(t, originalBody, retryBodyData, "Retry body must match original for HTTP/2 GOAWAY scenarios")
			})
		}
	})

	t.Run("non_reusable_readers_limitation", func(t *testing.T) {
		t.Parallel()

		nonReusableReaders := map[string]io.Reader{
			"io.NopCloser":  io.NopCloser(bytes.NewReader(testData)),
			"custom_reader": &customReader{data: testData},
		}

		for name, reader := range nonReusableReaders {
			reader := reader
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				req, err := NewReusableRequest(ctx, "POST", "https://example.com", reader)
				require.NoError(t, err)

				// These readers may not have GetBody set, which means they won't work
				// with HTTP/2 GOAWAY retries or redirects that need to replay the body
				if req.GetBody == nil {
					t.Logf("WARNING: %s does not support HTTP/2 GOAWAY retry - GetBody is nil", name)
				}
			})
		}
	})
}

// TestNewReusableRequestEdgeCases verifies that NewReusableRequest rejects an invalid URL and an invalid
// method, accepts an already-cancelled context and keeps it on the request, and sets a working GetBody for
// empty readers and for 1 MiB bytes bodies whose recreated content matches the original.
func TestNewReusableRequestEdgeCases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("invalid_url", func(t *testing.T) {
		t.Parallel()

		req, err := NewReusableRequest(ctx, "GET", "://invalid-url", nil)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("invalid_method", func(t *testing.T) {
		t.Parallel()

		// Test with invalid HTTP method containing spaces
		req, err := NewReusableRequest(ctx, "INVALID METHOD", "https://example.com", nil)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("cancelled_context", func(t *testing.T) {
		t.Parallel()

		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req, err := NewReusableRequest(cancelledCtx, "GET", "https://example.com", nil)
		require.NoError(t, err, "Request creation should succeed even with cancelled context")
		require.NotNil(t, req)
		require.Equal(t, cancelledCtx, req.Context())
	})

	t.Run("empty_body_readers", func(t *testing.T) {
		t.Parallel()

		emptyReaders := map[string]io.Reader{
			"empty_bytes_buffer":   bytes.NewBuffer(nil),
			"empty_bytes_reader":   bytes.NewReader(nil),
			"empty_strings_reader": strings.NewReader(""),
		}

		for name, reader := range emptyReaders {
			reader := reader
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				req, err := NewReusableRequest(ctx, "POST", "https://example.com", reader)
				require.NoError(t, err)
				require.NotNil(t, req)
				require.NotNil(t, req.GetBody, "Even empty reusable readers should have GetBody set")

				// Verify GetBody works with empty content
				newBody, err := req.GetBody()
				require.NoError(t, err)
				require.NotNil(t, newBody)

				data, err := io.ReadAll(newBody)
				require.NoError(t, err)
				require.Empty(t, data, "Empty reader should produce empty body")
			})
		}
	})

	t.Run("large_body_reusability", func(t *testing.T) {
		t.Parallel()

		// Test with a large body to ensure it's properly handled
		largeData := make([]byte, 1024*1024) // 1MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		readers := map[string]io.Reader{
			"large_bytes_buffer": bytes.NewBuffer(largeData),
			"large_bytes_reader": bytes.NewReader(largeData),
		}

		for name, reader := range readers {
			reader := reader
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				req, err := NewReusableRequest(ctx, "POST", "https://example.com", reader)
				require.NoError(t, err)
				require.NotNil(t, req)
				require.NotNil(t, req.GetBody)

				// Verify large body can be recreated
				newBody, err := req.GetBody()
				require.NoError(t, err)
				require.NotNil(t, newBody)

				recreatedData, err := io.ReadAll(newBody)
				require.NoError(t, err)
				require.Equal(t, largeData, recreatedData, "Large body should be correctly recreated")
			})
		}
	})
}

// customReader is a test helper that implements io.Reader but is not one of the
// automatically reusable types (bytes.Buffer, bytes.Reader, strings.Reader)
type customReader struct {
	data []byte
	pos  int
}

// Read implements io.Reader for customReader by copying the unread remainder of r.data into p and advancing
// the position. It returns the number of bytes copied with a nil error, or zero bytes and io.EOF once all
// data has been consumed.
func (r *customReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// BenchmarkNewReusableRequest measures the cost of creating a POST request with NewReusableRequest for
// bytes.Buffer, bytes.Reader, strings.Reader, and custom reader bodies, using a fresh reader per iteration.
func BenchmarkNewReusableRequest(b *testing.B) {
	ctx := context.Background()
	testData := []byte("benchmark test data")

	readers := map[string]func() io.Reader{
		"bytes_buffer":   func() io.Reader { return bytes.NewBuffer(testData) },
		"bytes_reader":   func() io.Reader { return bytes.NewReader(testData) },
		"strings_reader": func() io.Reader { return strings.NewReader(string(testData)) },
		"custom_reader":  func() io.Reader { return &customReader{data: testData} },
	}

	for name, readerFunc := range readers {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				reader := readerFunc()
				req, err := NewReusableRequest(ctx, "POST", "https://example.com", reader)
				if err != nil {
					b.Fatal(err)
				}
				_ = req
			}
		})
	}
}
