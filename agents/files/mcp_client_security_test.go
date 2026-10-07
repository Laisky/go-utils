package files

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

const (
	// syntheticMCPKey is a fake bearer credential used only with in-memory transports.
	syntheticMCPKey = "SYNTHETIC_KEY"
	// syntheticMCPEndpoint is a non-resolvable HTTPS endpoint used by the security tests.
	syntheticMCPEndpoint = "https://agent.example.invalid/mcp"
	// maxRecordedBodyBytes bounds how much of a request body the recorder keeps.
	maxRecordedBodyBytes = 64 * 1024
)

// errRecorderRejected is returned by recordingTransport when no responder is
// configured, so a recorded request is rejected without ever dialing.
var errRecorderRejected = errors.New("recording transport: request rejected without dialing")

// recordedRequest captures the security-relevant parts of one outbound request.
type recordedRequest struct {
	scheme        string
	host          string
	path          string
	authorization string
	body          string
}

// String renders the recorded request compactly for assertion messages.
func (r recordedRequest) String() string {
	return fmt.Sprintf("%s://%s%s auth=%q", r.scheme, r.host, r.path, r.authorization)
}

// recordingTransport is an in-memory http.RoundTripper and HTTPDoer. It
// records every request and answers through respond, or rejects the request
// when respond is nil. It never opens a socket.
type recordingTransport struct {
	respond func(req *http.Request, body []byte) (*http.Response, error)

	mu   sync.Mutex
	reqs []recordedRequest
}

// RoundTrip records req and returns the scripted response or a rejection.
func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(io.LimitReader(req.Body, maxRecordedBodyBytes))
		_ = req.Body.Close()
		if err != nil {
			return nil, errors.Wrap(err, "read recorded request body")
		}
		body = b
	}

	rt.mu.Lock()
	rt.reqs = append(rt.reqs, recordedRequest{
		scheme:        req.URL.Scheme,
		host:          req.URL.Host,
		path:          req.URL.Path,
		authorization: req.Header.Get("Authorization"),
		body:          string(body),
	})
	rt.mu.Unlock()

	if rt.respond == nil {
		return nil, errRecorderRejected
	}
	return rt.respond(req, body)
}

// Do implements HTTPDoer by delegating to RoundTrip, so the recorder can also
// stand in for a caller-supplied client.
func (rt *recordingTransport) Do(req *http.Request) (*http.Response, error) {
	return rt.RoundTrip(req)
}

// requests returns a copy of every request recorded so far.
func (rt *recordingTransport) requests() []recordedRequest {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]recordedRequest(nil), rt.reqs...)
}

// newScriptedResponse builds an in-memory HTTP response for req with the given
// status, extra headers, and body.
func newScriptedResponse(req *http.Request, status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

// mcpRedirectResponder returns a responder that redirects requests for the
// original endpoint path to location (307, so the body is replayed) and
// answers any other request with a valid MCP initialize or tools/call result.
func mcpRedirectResponder(location string) func(req *http.Request, body []byte) (*http.Response, error) {
	return func(req *http.Request, body []byte) (*http.Response, error) {
		if req.URL.Path == "/mcp" {
			return newScriptedResponse(req, http.StatusTemporaryRedirect,
				http.Header{"Location": []string{location}}, ""), nil
		}
		return mcpSuccessResponse(req, body), nil
	}
}

// mcpSuccessResponse answers an MCP JSON-RPC request: initialize returns a
// session id, and every other method returns an empty JSON object result.
func mcpSuccessResponse(req *http.Request, body []byte) *http.Response {
	var rpc struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &rpc)

	header := http.Header{"Content-Type": []string{"application/json"}}
	if rpc.Method == "initialize" {
		header.Set("Mcp-Session-Id", "synthetic-session")
	}
	return newScriptedResponse(req, http.StatusOK, header, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":"{}"}]}}`, rpc.ID))
}

// callSyntheticTool performs one bounded CallTool with synthetic arguments.
func callSyntheticTool(client *MCPClient) error {
	return client.CallTool(context.Background(), "file_stat",
		map[string]any{"project": "synthetic", "path": "/marker"}, nil)
}

// TestNewMCPClientRejectsInsecureEndpoints verifies that cleartext, malformed,
// userinfo-bearing, and non-HTTPS endpoints are rejected at construction and
// that no credential-bearing request is ever emitted. It is the reproduction
// for issue #47.
func TestNewMCPClientRejectsInsecureEndpoints(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{
		"http://agent.example.invalid/mcp",
		"HTTP://agent.example.invalid/mcp",
		"http://127.0.0.1:8080/mcp",
		"https://user:pass@agent.example.invalid/mcp",
		"https://user@agent.example.invalid/mcp",
		"user:pass@agent.example.invalid/mcp",
		"ftp://agent.example.invalid/mcp",
		"ws://agent.example.invalid/mcp",
		"file:///etc/passwd",
		"https:///mcp",
		"https://:443/mcp",
		"https://agent.example.invalid:99999/mcp",
		"https://agent.example.invalid:0/mcp",
		"https://agent example.invalid/mcp",
		"javascript:alert(1)",
	} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()

			rec := &recordingTransport{}
			client, err := NewMCPClient(MCPClientConfig{Endpoint: endpoint, APIKey: syntheticMCPKey, Client: rec})
			if err == nil {
				_ = callSyntheticTool(client)
			}
			require.Empty(t, rec.requests(), "credential-bearing request emitted for rejected endpoint")
			require.Error(t, err, "endpoint must be rejected at construction")
			require.NotContains(t, err.Error(), "pass@", "errors must not echo URL userinfo")
		})
	}
}

// TestNewMCPClientAcceptsHTTPSEndpoints verifies that HTTPS endpoints are
// accepted and that a scheme-less endpoint is normalized to HTTPS before the
// bearer credential is attached. It is a compatible-path regression for issue
// #47.
func TestNewMCPClientAcceptsHTTPSEndpoints(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		endpoint string
		wantHost string
	}{
		{endpoint: syntheticMCPEndpoint, wantHost: "agent.example.invalid"},
		{endpoint: "  HTTPS://agent.example.invalid/mcp  ", wantHost: "agent.example.invalid"},
		{endpoint: "agent.example.invalid/mcp", wantHost: "agent.example.invalid"},
		{endpoint: "//agent.example.invalid/mcp", wantHost: "agent.example.invalid"},
		{endpoint: "agent.example.invalid:8443/mcp", wantHost: "agent.example.invalid:8443"},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			t.Parallel()

			rec := &recordingTransport{}
			client, err := NewMCPClient(MCPClientConfig{Endpoint: tc.endpoint, APIKey: syntheticMCPKey, Client: rec})
			require.NoError(t, err)
			require.Error(t, callSyntheticTool(client))

			reqs := rec.requests()
			require.Len(t, reqs, 1)
			require.Equal(t, "https", reqs[0].scheme)
			require.Equal(t, tc.wantHost, reqs[0].host)
			require.Equal(t, "/mcp", reqs[0].path)
			require.Equal(t, "Bearer "+syntheticMCPKey, reqs[0].authorization)
		})
	}
}

// TestMCPClientDefaultClientRefusesUnsafeRedirects verifies that the default
// HTTP client built by NewMCPClient refuses HTTPS-to-HTTP downgrades and
// cross-origin redirects, so neither the bearer credential nor the replayed
// JSON-RPC payload reaches another origin. It is the redirect reproduction for
// issue #47.
func TestMCPClientDefaultClientRefusesUnsafeRedirects(t *testing.T) {
	t.Parallel()

	for _, location := range []string{
		"http://agent.example.invalid/mcp",
		"http://agent.example.invalid:443/v2",
		"https://other.example.invalid/mcp",
		"https://evil.agent.example.invalid/mcp",
		"https://agent.example.invalid:8443/mcp",
	} {
		t.Run(location, func(t *testing.T) {
			t.Parallel()

			client, err := NewMCPClient(MCPClientConfig{Endpoint: syntheticMCPEndpoint, APIKey: syntheticMCPKey})
			require.NoError(t, err)
			httpCli, ok := client.httpCli.(*http.Client)
			require.True(t, ok)
			rec := &recordingTransport{respond: mcpRedirectResponder(location)}
			httpCli.Transport = rec

			callErr := callSyntheticTool(client)
			require.Len(t, rec.requests(), 1, "redirect must not be followed; requests: %v", rec.requests())
			require.Error(t, callErr)
			require.Contains(t, callErr.Error(), "redirect")
		})
	}
}

// TestMCPClientDefaultClientFollowsSameOriginRedirect verifies that a redirect
// that stays on the original HTTPS origin is still followed with the bearer
// credential, keeping legitimate deployments working. It is a compatible-path
// regression for issue #47.
func TestMCPClientDefaultClientFollowsSameOriginRedirect(t *testing.T) {
	t.Parallel()

	client, err := NewMCPClient(MCPClientConfig{Endpoint: syntheticMCPEndpoint, APIKey: syntheticMCPKey})
	require.NoError(t, err)
	httpCli, ok := client.httpCli.(*http.Client)
	require.True(t, ok)
	rec := &recordingTransport{respond: mcpRedirectResponder("https://agent.example.invalid/mcp/v2")}
	httpCli.Transport = rec

	require.NoError(t, callSyntheticTool(client))
	require.Greater(t, len(rec.requests()), 1, "same-origin redirect must be followed")
	for _, req := range rec.requests() {
		require.Equal(t, "https", req.scheme)
		require.Equal(t, "Bearer "+syntheticMCPKey, req.authorization)
	}
}

// TestMCPClientDefaultClientFollowsCanonicallySameOriginRedirect verifies that
// a redirect whose host differs only in letter case or an explicit default
// port is treated as the same origin and followed over HTTPS. Whether
// net/http re-sends the Authorization header for such a spelling differs
// between Go releases (1.26 drops it, 1.27 keeps it); both are safe, so the
// credential is deliberately not asserted here.
func TestMCPClientDefaultClientFollowsCanonicallySameOriginRedirect(t *testing.T) {
	t.Parallel()

	client, err := NewMCPClient(MCPClientConfig{Endpoint: syntheticMCPEndpoint, APIKey: syntheticMCPKey})
	require.NoError(t, err)
	httpCli, ok := client.httpCli.(*http.Client)
	require.True(t, ok)
	rec := &recordingTransport{respond: mcpRedirectResponder("https://AGENT.example.invalid:443/mcp/v2")}
	httpCli.Transport = rec

	callErr := callSyntheticTool(client)
	if callErr != nil {
		require.NotContains(t, callErr.Error(), "refusing", "canonically same-origin redirect must not be refused")
	}
	require.Greater(t, len(rec.requests()), 1, "canonically same-origin redirect must be followed")
	for _, req := range rec.requests() {
		require.Equal(t, "https", req.scheme)
	}
}

// TestNewMCPClientAllowInsecureHTTPOptIn verifies that cleartext http is only
// accepted through the explicit AllowInsecureHTTP opt-in, that the opt-in does
// not relax the other URL checks, and that it carries no implicit loopback
// exception. It is the opt-in regression for issue #47.
func TestNewMCPClientAllowInsecureHTTPOptIn(t *testing.T) {
	t.Parallel()

	t.Run("http accepted with opt-in", func(t *testing.T) {
		t.Parallel()

		rec := &recordingTransport{}
		client, err := NewMCPClient(MCPClientConfig{
			Endpoint:          "http://127.0.0.1:8080/mcp",
			APIKey:            syntheticMCPKey,
			Client:            rec,
			AllowInsecureHTTP: true,
		})
		require.NoError(t, err)
		require.Error(t, callSyntheticTool(client))

		reqs := rec.requests()
		require.Len(t, reqs, 1)
		require.Equal(t, "http", reqs[0].scheme)
		require.Equal(t, "127.0.0.1:8080", reqs[0].host)
	})

	t.Run("loopback http rejected without opt-in", func(t *testing.T) {
		t.Parallel()

		for _, endpoint := range []string{"http://127.0.0.1:8080/mcp", "http://localhost/mcp", "http://[::1]/mcp"} {
			_, err := NewMCPClient(MCPClientConfig{Endpoint: endpoint, APIKey: syntheticMCPKey})
			require.Error(t, err, endpoint)
		}
	})

	t.Run("opt-in keeps other checks", func(t *testing.T) {
		t.Parallel()

		for _, endpoint := range []string{
			"http://user:pass@127.0.0.1/mcp",
			"ftp://127.0.0.1/mcp",
			"http:///mcp",
		} {
			rec := &recordingTransport{}
			_, err := NewMCPClient(MCPClientConfig{
				Endpoint: endpoint, APIKey: syntheticMCPKey, Client: rec, AllowInsecureHTTP: true,
			})
			require.Error(t, err, endpoint)
			require.Empty(t, rec.requests())
		}
	})
}

// TestMCPClientOverTLSServer verifies the end-to-end HTTPS path against a
// local TLS test server whose client trusts the test certificate: the bearer
// credential is delivered and initialize plus tools/call succeed. It is a
// compatible-path regression for issue #47.
func TestMCPClientOverTLSServer(t *testing.T) {
	t.Parallel()

	var (
		mu    sync.Mutex
		auths []string
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRecordedBodyBytes))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()

		resp := mcpSuccessResponse(r, body)
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer server.Close()

	client, err := NewMCPClient(MCPClientConfig{Endpoint: server.URL + "/mcp", APIKey: syntheticMCPKey, Client: server.Client()})
	require.NoError(t, err)
	require.NoError(t, callSyntheticTool(client))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, auths, 2)
	for _, auth := range auths {
		require.Equal(t, "Bearer "+syntheticMCPKey, auth)
	}
}

// TestNewMCPStorageFromConfigRejectsCleartextEndpoint verifies that the
// endpoint-based storage constructor inherits the endpoint policy and fails
// before its bootstrap request. It is a regression for issue #47.
func TestNewMCPStorageFromConfigRejectsCleartextEndpoint(t *testing.T) {
	t.Parallel()

	_, err := NewMCPStorageFromConfig(context.Background(), "http://agent.example.invalid/mcp", syntheticMCPKey)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cleartext http")
}
