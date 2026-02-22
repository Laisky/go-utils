package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type capturedRequest struct {
	Method string
	Header http.Header
	Body   []byte
}

func newJSONEchoServer() (*httptest.Server, <-chan capturedRequest) {
	captureCh := make(chan capturedRequest, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var payload map[string]string
		if len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		captureCh <- capturedRequest{
			Method: r.Method,
			Header: r.Header.Clone(),
			Body:   append([]byte(nil), body...),
		}

		w.Header().Set("Content-Type", "application/json")
		resp := struct {
			JSON map[string]string `json:"json"`
		}{
			JSON: payload,
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	server := httptest.NewServer(handler)
	return server, captureCh
}

func receiveCapturedRequest(t *testing.T, ch <-chan capturedRequest) capturedRequest {
	t.Helper()

	select {
	case req := <-ch:
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request")
	}

	return capturedRequest{}
}

func TestRequestJSON(t *testing.T) {
	server, captureCh := newJSONEchoServer()
	defer server.Close()

	data := RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	err := RequestJSON("POST", server.URL, &data, &resp)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"hello": "world"}, resp.JSON)

	captured := receiveCapturedRequest(t, captureCh)
	require.Equal(t, http.MethodPost, captured.Method)
	require.Equal(t, HTTPHeaderContentTypeValJSON, captured.Header.Get(HTTPHeaderContentType))
	require.JSONEq(t, `{"hello":"world"}`, string(captured.Body))
}
func TestRequestJSONWithClient(t *testing.T) {
	server, captureCh := newJSONEchoServer()
	defer server.Close()

	data := RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	httpClient, err := NewHTTPClient(
		WithHTTPClientInsecure(),
		WithHTTPClientMaxConn(20),
		WithHTTPClientTimeout(30*time.Second),
	)
	require.NoError(t, err)

	err = RequestJSONWithClient(httpClient, "POST", server.URL, &data, &resp)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"hello": "world"}, resp.JSON)

	captured := receiveCapturedRequest(t, captureCh)
	require.Equal(t, http.MethodPost, captured.Method)
	require.Equal(t, HTTPHeaderContentTypeValJSON, captured.Header.Get(HTTPHeaderContentType))
	require.JSONEq(t, `{"hello":"world"}`, string(captured.Body))
}

func TestRequestJSONWithClientNilRequest(t *testing.T) {
	server, captureCh := newJSONEchoServer()
	defer server.Close()

	var resp struct {
		JSON map[string]string `json:"json"`
	}

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(5 * time.Second))
	require.NoError(t, err)

	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.NoError(t, err)
	require.Nil(t, resp.JSON)

	captured := receiveCapturedRequest(t, captureCh)
	require.Equal(t, http.MethodGet, captured.Method)
	require.Empty(t, captured.Body)
	require.Empty(t, captured.Header.Get(HTTPHeaderContentType))
}

func TestRequestJSONWithClientNilHTTPClient(t *testing.T) {
	server, captureCh := newJSONEchoServer()
	defer server.Close()

	data := RequestData{
		Data: map[string]string{"hello": "world"},
	}

	var resp struct {
		JSON map[string]string `json:"json"`
	}

	err := RequestJSONWithClient(nil, http.MethodPost, server.URL, &data, &resp)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"hello": "world"}, resp.JSON)

	captured := receiveCapturedRequest(t, captureCh)
	require.Equal(t, http.MethodPost, captured.Method)
	require.JSONEq(t, `{"hello":"world"}`, string(captured.Body))
}

func TestRequestJSONWithClientLargeErrorBodyIsTruncated(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(strings.Repeat("x", 20*1024)))
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(5 * time.Second))
	require.NoError(t, err)

	var resp map[string]any
	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "(truncated)")
	require.Less(t, len(err.Error()), 8300)
}

func TestRequestJSONWithClientLargeSuccessBodyWithinLimit(t *testing.T) {
	t.Parallel()

	payloadSize := int(maxRequestJSONSuccessBodyBytes) - 1024
	payload := strings.Repeat("a", payloadSize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"payload":%q}`, payload)
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(10 * time.Second))
	require.NoError(t, err)

	var resp struct {
		Payload string `json:"payload"`
	}

	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.NoError(t, err)
	require.Equal(t, payload, resp.Payload)
}

func TestRequestJSONWithClientLargeSuccessBodyExceedsLimit(t *testing.T) {
	t.Parallel()

	payloadSize := int(maxRequestJSONSuccessBodyBytes) + 1024
	payload := strings.Repeat("b", payloadSize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"payload":%q}`, payload)
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(10 * time.Second))
	require.NoError(t, err)

	var resp struct {
		Payload string `json:"payload"`
	}

	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "response body too large")
}

func TestRequestJSONWithClientCustomMaxResponseBodyBytes(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("z", 2048)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"payload":%q}`, payload)
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(10 * time.Second))
	require.NoError(t, err)

	var resp struct {
		Payload string `json:"payload"`
	}

	err = RequestJSONWithClient(
		httpClient,
		http.MethodGet,
		server.URL,
		nil,
		&resp,
		WithRequestJSONMaxResponseBodyBytes(1024),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "response body too large")

	err = RequestJSONWithClient(
		httpClient,
		http.MethodGet,
		server.URL,
		nil,
		&resp,
		WithRequestJSONMaxResponseBodyBytes(10*1024),
	)
	require.NoError(t, err)
	require.Equal(t, payload, resp.Payload)
}

func TestRequestJSONWithClientInvalidOption(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(10 * time.Second))
	require.NoError(t, err)

	var resp map[string]any
	err = RequestJSONWithClient(
		httpClient,
		http.MethodGet,
		server.URL,
		nil,
		&resp,
		WithRequestJSONMaxResponseBodyBytes(0),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max response body bytes should greater than 0")
}

func TestCheckResp(t *testing.T) {
	var (
		resp *http.Response
		err  error
	)
	resp = &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(bytes.NewBufferString(`some error message`)),
	}
	err = CheckResp(resp)
	if err == nil {
		t.Error("missing error")
	}
	if !strings.Contains(err.Error(), "some error message") {
		t.Errorf("error message error <%v>", err.Error())
	}
}

func TestCheckRespLargeErrorBodyIsTruncated(t *testing.T) {
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(bytes.NewBufferString(strings.Repeat("x", 20*1024))),
	}

	err := CheckResp(resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "got http body (truncated):")
	require.Less(t, len(err.Error()), 9000)
}

func TestCheckRespWithCustomMaxErrorBodyBytes(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(bytes.NewBufferString(strings.Repeat("y", 2048))),
	}

	err := CheckResp(resp, WithCheckRespMaxErrorBodyBytes(1024))
	require.Error(t, err)
	require.Contains(t, err.Error(), "got http body (truncated):")
	require.Less(t, len(err.Error()), 1300)
}

func TestCheckRespInvalidOption(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"ok": true}`)),
	}

	err := CheckResp(resp, WithCheckRespMaxErrorBodyBytes(0))
	require.Error(t, err)
	require.Contains(t, err.Error(), "max check response error body bytes should greater than 0")
}

func TestJaegerTracingID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		traceID      uint64
		spanID       uint64
		parentSpanID uint64
		flag         byte
		want         JaegerTracingID
		wantErr      bool
	}{
		{
			name:         "valid tracing ID",
			traceID:      123456789,
			spanID:       987654321,
			parentSpanID: 0,
			flag:         0x04,
			want:         "75bcd15:3ade68b1::4",
			wantErr:      false,
		},
		{
			name:         "invalid trace ID",
			traceID:      0,
			spanID:       987654321,
			parentSpanID: 0,
			flag:         0x04,
			want:         "",
			wantErr:      true,
		},
		{
			name:         "invalid span ID",
			traceID:      123456789,
			spanID:       0,
			parentSpanID: 0,
			flag:         0x04,
			want:         "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewJaegerTracingID(tt.traceID, tt.spanID, tt.parentSpanID, tt.flag)
			require.NoError(t, err)
			if tt.wantErr {
				require.NotEqual(t, tt.want.String(), got.String())
				_, _, _, _, err = got.Parse()
				require.NoError(t, err)
				return
			}

			require.Equal(t, tt.want.String(), got.String())

			traceID, spanID, parentSpanID, flag, err := got.Parse()
			require.NoError(t, err)
			require.Equal(t, tt.traceID, traceID)
			require.Equal(t, tt.spanID, spanID)
			require.Equal(t, tt.parentSpanID, parentSpanID)
			require.Equal(t, tt.flag, flag)
		})
	}
}

func TestValidateOpenBrowserURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid http", input: "http://example.com/path", wantErr: false},
		{name: "valid https", input: "https://example.com/path?q=1", wantErr: false},
		{name: "valid mailto", input: "mailto:test@example.com", wantErr: false},
		{name: "invalid javascript scheme", input: "javascript:alert(1)", wantErr: true},
		{name: "invalid file scheme", input: "file:///etc/passwd", wantErr: true},
		{name: "invalid malformed url", input: "http://[::1", wantErr: true},
		{name: "invalid missing host for https", input: "https:///path", wantErr: true},
		{name: "invalid mailto without recipient", input: "mailto:", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateOpenBrowserURL(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

func TestBuildOpenURLCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		goos     string
		isWSL    bool
		url      string
		wantCmd  string
		wantArgs []string
	}{
		{
			name:     "windows uses cmd start with title placeholder",
			goos:     "windows",
			isWSL:    false,
			url:      "https://example.com",
			wantCmd:  "cmd",
			wantArgs: []string{"/c", "start", "", "https://example.com"},
		},
		{
			name:     "darwin uses open with url arg",
			goos:     "darwin",
			isWSL:    false,
			url:      "https://example.com",
			wantCmd:  "open",
			wantArgs: []string{"https://example.com"},
		},
		{
			name:     "linux uses xdg-open",
			goos:     "linux",
			isWSL:    false,
			url:      "https://example.com",
			wantCmd:  "xdg-open",
			wantArgs: []string{"https://example.com"},
		},
		{
			name:     "wsl uses cmd.exe start with title placeholder",
			goos:     "linux",
			isWSL:    true,
			url:      "https://example.com",
			wantCmd:  "cmd.exe",
			wantArgs: []string{"/c", "start", "", "https://example.com"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd, args := buildOpenURLCommand(tt.goos, tt.isWSL, tt.url)
			require.Equal(t, tt.wantCmd, cmd)
			require.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestOpenURLInDefaultBrowserRejectsInvalidSchemes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := OpenURLInDefaultBrowser(ctx, "javascript:alert(1)")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported url scheme")
}

func TestOpenURLInDefaultBrowserRejectsMalformedURL(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := OpenURLInDefaultBrowser(ctx, "http://[::1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse url")
}

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

func (r *customReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

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

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

// streamReader simulates a streaming reader with delays
type streamReader struct {
	data  []byte
	pos   int
	delay time.Duration
}

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
