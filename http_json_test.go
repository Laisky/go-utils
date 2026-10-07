package utils

import (
	"bytes"
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

// httpTestClientTimeout is deliberately generous: these tests exercise body
// limits and error formatting, not timeouts, and they run in parallel under
// the race detector where large payloads can be slow on a loaded host.
const httpTestClientTimeout = 2 * time.Minute

type capturedRequest struct {
	Method string
	Header http.Header
	Body   []byte
}

// newJSONEchoServer starts an httptest server that decodes a JSON object request body, records the method,
// headers, and raw body on the returned buffered channel (capacity 1), and responds with {"json": <payload>}.
// It returns the running server, which the caller must close, and the receive-only capture channel.
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

// receiveCapturedRequest waits up to two seconds for a request recorded by newJSONEchoServer on ch and
// returns it. t is the calling test, which is failed fatally if no request arrives before the timeout.
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

// TestRequestJSON verifies that RequestJSON with the internal client POSTs the request data as a JSON body
// with the application/json Content-Type header and decodes the echoed JSON response into the result.
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

// TestRequestJSONWithClient verifies that RequestJSONWithClient using a custom client (insecure, 20 max
// connections, 30-second timeout) POSTs the data as JSON with the JSON Content-Type and decodes the echo.
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

// TestRequestJSONWithClientNilRequest verifies that a nil request makes RequestJSONWithClient send a GET
// with an empty body and no Content-Type header, and that the echoed null payload decodes to a nil map.
func TestRequestJSONWithClientNilRequest(t *testing.T) {
	server, captureCh := newJSONEchoServer()
	defer server.Close()

	var resp struct {
		JSON map[string]string `json:"json"`
	}

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(httpTestClientTimeout))
	require.NoError(t, err)

	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.NoError(t, err)
	require.Nil(t, resp.JSON)

	captured := receiveCapturedRequest(t, captureCh)
	require.Equal(t, http.MethodGet, captured.Method)
	require.Empty(t, captured.Body)
	require.Empty(t, captured.Header.Get(HTTPHeaderContentType))
}

// TestRequestJSONWithClientNilHTTPClient verifies that RequestJSONWithClient falls back to the internal client
// when given a nil *http.Client, still sending the JSON body and decoding the echoed response.
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

// TestRequestJSONWithClientLargeErrorBodyIsTruncated verifies that a 400 response with a 20 KiB body yields an
// error marked "(truncated)" whose message stays below 8300 bytes because of the default error-body limit.
func TestRequestJSONWithClientLargeErrorBodyIsTruncated(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(strings.Repeat("x", 20*1024)))
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(httpTestClientTimeout))
	require.NoError(t, err)

	var resp map[string]any
	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "(truncated)")
	require.Less(t, len(err.Error()), 8300)
}

// TestRequestJSONWithClientLargeSuccessBodyWithinLimit verifies that a successful JSON response just under the
// default success-body limit (8 MiB minus 1 KiB of payload) is read and decoded in full.
func TestRequestJSONWithClientLargeSuccessBodyWithinLimit(t *testing.T) {
	t.Parallel()

	payloadSize := int(maxRequestJSONSuccessBodyBytes) - 1024
	payload := strings.Repeat("a", payloadSize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"payload":%q}`, payload)
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(httpTestClientTimeout))
	require.NoError(t, err)

	var resp struct {
		Payload string `json:"payload"`
	}

	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.NoError(t, err)
	require.Equal(t, payload, resp.Payload)
}

// TestRequestJSONWithClientLargeSuccessBodyExceedsLimit verifies that a successful JSON response larger than
// the default 8 MiB success-body limit is rejected with a "response body too large" error.
func TestRequestJSONWithClientLargeSuccessBodyExceedsLimit(t *testing.T) {
	t.Parallel()

	payloadSize := int(maxRequestJSONSuccessBodyBytes) + 1024
	payload := strings.Repeat("b", payloadSize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"payload":%q}`, payload)
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(httpTestClientTimeout))
	require.NoError(t, err)

	var resp struct {
		Payload string `json:"payload"`
	}

	err = RequestJSONWithClient(httpClient, http.MethodGet, server.URL, nil, &resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "response body too large")
}

// TestRequestJSONWithClientCustomMaxResponseBodyBytes verifies that WithRequestJSONMaxResponseBodyBytes
// overrides the success-body limit: a 2 KiB payload fails with "response body too large" under a 1 KiB limit
// and decodes successfully under a 10 KiB limit.
func TestRequestJSONWithClientCustomMaxResponseBodyBytes(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("z", 2048)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"payload":%q}`, payload)
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(httpTestClientTimeout))
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

// TestRequestJSONWithClientInvalidOption verifies that RequestJSONWithClient returns an option error when
// WithRequestJSONMaxResponseBodyBytes is given a non-positive limit of 0.
func TestRequestJSONWithClientInvalidOption(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	httpClient, err := NewHTTPClient(WithHTTPClientTimeout(httpTestClientTimeout))
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

// TestCheckResp verifies that CheckResp returns an error for a 500 response and that the error message
// includes the response body text.
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

// TestCheckRespLargeErrorBodyIsTruncated verifies that CheckResp truncates a 20 KiB error body to the default
// limit, marks the error with "got http body (truncated):", and keeps the message below 9000 bytes.
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

// TestCheckRespWithCustomMaxErrorBodyBytes verifies that WithCheckRespMaxErrorBodyBytes(1024) truncates a
// 2 KiB error body, marks the error as truncated, and keeps the message below 1300 bytes.
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

// TestCheckRespInvalidOption verifies that CheckResp returns an option error, even for a 200 response, when
// WithCheckRespMaxErrorBodyBytes is given a non-positive limit of 0.
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
