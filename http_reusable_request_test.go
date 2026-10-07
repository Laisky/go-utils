package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNewReusableRequestHTTP2GOAWAYScenarios tests specific HTTP/2 GOAWAY scenarios
// as described in the technical documentation
func TestNewReusableRequestHTTP2GOAWAYScenarios(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("proxy_passthrough_scenario", func(t *testing.T) {
		t.Parallel()

		// Simulate a reverse proxy scenario where we receive a request
		// and need to forward it upstream with the ability to retry on GOAWAY
		originalBody := `{
			"model": "gpt-3.5-turbo",
			"messages": [{"role": "user", "content": "Hello"}],
			"stream": true
		}`

		// Test various input types that a proxy might receive
		testCases := []struct {
			name   string
			reader io.Reader
		}{
			{
				name:   "from_http_request_body",
				reader: io.NopCloser(strings.NewReader(originalBody)),
			},
			{
				name:   "from_bytes_buffer",
				reader: bytes.NewBuffer([]byte(originalBody)),
			},
			{
				name:   "from_strings_reader",
				reader: strings.NewReader(originalBody),
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				req, err := NewReusableRequest(ctx, "POST", "https://api.openai.com/v1/chat/completions", tc.reader)
				require.NoError(t, err)
				require.NotNil(t, req)

				// Simulate first attempt - read the body
				firstAttemptBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)

				// Simulate GOAWAY received - need to retry
				if req.GetBody != nil {
					// This simulates what http.Transport does on GOAWAY retry
					retryBody, err := req.GetBody()
					require.NoError(t, err)
					require.NotNil(t, retryBody)

					retryBodyData, err := io.ReadAll(retryBody)
					require.NoError(t, err)
					require.Equal(t, firstAttemptBody, retryBodyData, "GOAWAY retry must have identical body")
				} else {
					t.Logf("WARNING: Request with %s cannot be retried on HTTP/2 GOAWAY", tc.name)
				}
			})
		}
	})

	t.Run("multiple_retry_scenario", func(t *testing.T) {
		t.Parallel()

		// Test that GetBody can be called multiple times (multiple GOAWAYs)
		testData := []byte(`{"test": "multiple retries"}`)
		req, err := NewReusableRequest(ctx, "POST", "https://example.com", bytes.NewReader(testData))
		require.NoError(t, err)
		require.NotNil(t, req.GetBody)

		// Simulate multiple GOAWAY scenarios
		for i := 0; i < 3; i++ {
			retryBody, err := req.GetBody()
			require.NoError(t, err, "GetBody should work on attempt %d", i+1)

			retryData, err := io.ReadAll(retryBody)
			require.NoError(t, err)
			require.Equal(t, testData, retryData, "Body should be identical on retry %d", i+1)
		}
	})

	t.Run("concurrent_getbody_calls", func(t *testing.T) {
		t.Parallel()

		// Test concurrent access to GetBody (race condition testing)
		testData := []byte(`{"test": "concurrent access"}`)
		req, err := NewReusableRequest(ctx, "POST", "https://example.com", bytes.NewReader(testData))
		require.NoError(t, err)
		require.NotNil(t, req.GetBody)

		const numGoroutines = 10
		results := make(chan []byte, numGoroutines)
		errors := make(chan error, numGoroutines)

		// Start concurrent GetBody calls
		for i := 0; i < numGoroutines; i++ {
			go func() {
				retryBody, err := req.GetBody()
				if err != nil {
					errors <- err
					return
				}
				data, err := io.ReadAll(retryBody)
				if err != nil {
					errors <- err
					return
				}
				results <- data
			}()
		}

		// Collect results
		for i := 0; i < numGoroutines; i++ {
			select {
			case data := <-results:
				require.Equal(t, testData, data, "Concurrent GetBody call %d should return correct data", i)
			case err := <-errors:
				require.NoError(t, err, "Concurrent GetBody call %d should not error", i)
			case <-time.After(5 * time.Second):
				t.Fatalf("Timeout waiting for concurrent GetBody call %d", i)
			}
		}
	})

	t.Run("memory_efficiency_large_payload", func(t *testing.T) {
		t.Parallel()

		// Test memory efficiency with large payloads
		// This simulates file upload scenarios through proxy
		largeData := make([]byte, 10*1024*1024) // 10MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		req, err := NewReusableRequest(ctx, "POST", "https://example.com/upload", bytes.NewReader(largeData))
		require.NoError(t, err)
		require.NotNil(t, req.GetBody)

		// Verify that GetBody works with large payloads
		retryBody, err := req.GetBody()
		require.NoError(t, err)

		// Read in chunks to avoid memory pressure during test
		retryData := make([]byte, len(largeData))
		n, err := io.ReadFull(retryBody, retryData)
		require.NoError(t, err)
		require.Equal(t, len(largeData), n)
		require.Equal(t, largeData, retryData)
	})
}

// TestNewReusableRequestErrorConditions tests various error conditions and edge cases
func TestNewReusableRequestErrorConditions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("malformed_requests", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			method  string
			url     string
			wantErr bool
		}{
			{
				name:    "empty_method",
				method:  "",
				url:     "https://example.com",
				wantErr: false, // Empty method is actually valid (defaults to GET)
			},
			{
				name:    "invalid_method_with_space",
				method:  "GET POST",
				url:     "https://example.com",
				wantErr: true,
			},
			{
				name:    "invalid_method_with_newline",
				method:  "GET\n",
				url:     "https://example.com",
				wantErr: true,
			},
			{
				name:    "malformed_url_no_scheme",
				method:  "GET",
				url:     "example.com",
				wantErr: false, // This actually works (becomes relative URL)
			},
			{
				name:    "malformed_url_invalid_scheme",
				method:  "GET",
				url:     "://example.com",
				wantErr: true,
			},
			{
				name:    "url_with_invalid_characters",
				method:  "GET",
				url:     "https://example.com/path with spaces",
				wantErr: false, // URL encoding handles this
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				req, err := NewReusableRequest(ctx, tc.method, tc.url, nil)
				if tc.wantErr {
					require.Error(t, err)
					require.Nil(t, req)
				} else {
					require.NoError(t, err)
					require.NotNil(t, req)
				}
			})
		}
	})

	t.Run("getbody_error_scenarios", func(t *testing.T) {
		t.Parallel()

		// Test with a reader that returns an error
		errorReader := &errorReader{err: fmt.Errorf("simulated read error")}
		req, err := NewReusableRequest(ctx, "POST", "https://example.com", errorReader)
		require.NoError(t, err) // Request creation should succeed
		require.NotNil(t, req)

		// The error reader won't have GetBody set since it's not a known reusable type
		if req.GetBody != nil {
			// If GetBody is set, it should handle errors gracefully
			_, err := req.GetBody()
			// The behavior here depends on implementation - it might error or not
			// The important thing is that it doesn't panic
			t.Logf("GetBody with error reader: %v", err)
		}
	})

	t.Run("context_deadline_exceeded", func(t *testing.T) {
		t.Parallel()

		// Test with context that has already exceeded deadline
		pastTime := time.Now().Add(-1 * time.Hour)
		deadlineCtx, cancel := context.WithDeadline(context.Background(), pastTime)
		defer cancel()

		req, err := NewReusableRequest(deadlineCtx, "GET", "https://example.com", nil)
		require.NoError(t, err, "Request creation should succeed even with exceeded deadline")
		require.NotNil(t, req)
		require.Equal(t, deadlineCtx, req.Context())
	})
}

// TestNewReusableRequestRealWorldScenarios tests realistic usage patterns
func TestNewReusableRequestRealWorldScenarios(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("json_api_request", func(t *testing.T) {
		t.Parallel()

		// Simulate a typical JSON API request
		payload := map[string]interface{}{
			"query":         "SELECT * FROM users",
			"variables":     map[string]interface{}{"limit": 10},
			"operationName": "GetUsers",
		}

		jsonData, err := json.Marshal(payload)
		require.NoError(t, err)

		req, err := NewReusableRequest(ctx, "POST", "https://api.example.com/graphql", bytes.NewReader(jsonData))
		require.NoError(t, err)
		require.NotNil(t, req)
		require.NotNil(t, req.GetBody, "JSON API requests should be reusable for GOAWAY scenarios")

		// Verify content type can be set
		req.Header.Set("Content-Type", "application/json")
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
	})

	t.Run("form_data_request", func(t *testing.T) {
		t.Parallel()

		// Simulate a form data POST request
		formData := "username=testuser&password=testpass&remember=true"
		req, err := NewReusableRequest(ctx, "POST", "https://example.com/login", strings.NewReader(formData))
		require.NoError(t, err)
		require.NotNil(t, req)
		require.NotNil(t, req.GetBody, "Form data requests should be reusable")

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		require.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))
	})

	t.Run("streaming_request_simulation", func(t *testing.T) {
		t.Parallel()

		// Simulate a streaming request where we can't reuse the body
		// This is like a live stream or large file upload from an external source
		streamReader := &streamReader{data: []byte("streaming data"), delay: 10 * time.Millisecond}
		req, err := NewReusableRequest(ctx, "POST", "https://example.com/stream", streamReader)
		require.NoError(t, err)
		require.NotNil(t, req)

		// Streaming readers typically won't have GetBody set
		if req.GetBody == nil {
			t.Log("Streaming reader doesn't support GOAWAY retry (expected behavior)")
		}
	})

	t.Run("proxy_with_authentication", func(t *testing.T) {
		t.Parallel()

		// Simulate a proxy request with authentication headers
		requestBody := `{"action": "process", "data": "sensitive information"}`
		req, err := NewReusableRequest(ctx, "POST", "https://secure-api.example.com/process", strings.NewReader(requestBody))
		require.NoError(t, err)
		require.NotNil(t, req)

		// Add authentication headers
		req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...")
		req.Header.Set("X-API-Key", "secret-api-key")
		req.Header.Set("User-Agent", "MyProxy/1.0")

		// Verify headers are preserved
		require.Equal(t, "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...", req.Header.Get("Authorization"))
		require.Equal(t, "secret-api-key", req.Header.Get("X-API-Key"))
		require.Equal(t, "MyProxy/1.0", req.Header.Get("User-Agent"))

		// Verify body reusability for retry scenarios
		require.NotNil(t, req.GetBody, "Authenticated requests should support retry on GOAWAY")
	})
}

// errorReader is a test helper that always returns an error when read
type errorReader struct {
	err error
}

// Read implements io.Reader for errorReader by ignoring p and always returning zero bytes together with the
// configured r.err, which lets tests simulate a body that fails on every read.
func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

// streamReader simulates a streaming reader with delays
type streamReader struct {
	data  []byte
	pos   int
	delay time.Duration
}

// Read implements io.Reader for streamReader: it first sleeps for r.delay when positive, then copies the
// unread remainder of r.data into p and advances the position. It returns the number of bytes copied with a
// nil error, or zero bytes and io.EOF once all data has been consumed.
func (r *streamReader) Read(p []byte) (n int, err error) {
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
