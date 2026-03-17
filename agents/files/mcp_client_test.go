package files

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockHTTPDoer returns configurable HTTP response for MCP client tests.
type mockHTTPDoer struct {
	resp *http.Response
	err  error
}

// Do implements HTTPDoer for deterministic unit testing.
func (doer *mockHTTPDoer) Do(_ *http.Request) (*http.Response, error) {
	if doer.err != nil {
		return nil, doer.err
	}

	return doer.resp, nil
}

// TestMCPClientDoRPCRejectsLargeBody verifies oversized RPC payloads are rejected.
func TestMCPClientDoRPCRejectsLargeBody(t *testing.T) {
	t.Parallel()

	client, err := NewMCPClient(MCPClientConfig{
		Endpoint: "https://example.com/mcp",
		APIKey:   "test-key",
		Client: &mockHTTPDoer{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("a", maxMCPRPCResponseBodyBytes+128))),
		}},
	})
	require.NoError(t, err)

	var out rpcResp
	err = client.doRPC(context.Background(), "", rpcReq{JSONRPC: jsonrpcVersion, ID: 1, Method: "initialize"}, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "(truncated)")
	require.Contains(t, err.Error(), "exceeds limit")
}

// TestMCPClientDoRPCWithinLimit verifies normal RPC payloads continue to work.
func TestMCPClientDoRPCWithinLimit(t *testing.T) {
	t.Parallel()

	body := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}`
	client, err := NewMCPClient(MCPClientConfig{
		Endpoint: "https://example.com/mcp",
		APIKey:   "test-key",
		Client: &mockHTTPDoer{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}},
	})
	require.NoError(t, err)

	var out rpcResp
	err = client.doRPC(context.Background(), "", rpcReq{JSONRPC: jsonrpcVersion, ID: 1, Method: "initialize"}, &out)
	require.NoError(t, err)
	require.Equal(t, int64(1), out.ID)
}

// TestNewMCPClientSetsDefaultTimeout verifies the fallback HTTP client enforces a timeout.
func TestNewMCPClientSetsDefaultTimeout(t *testing.T) {
	t.Parallel()

	client, err := NewMCPClient(MCPClientConfig{
		Endpoint: "https://example.com/mcp",
		APIKey:   "test-key",
	})
	require.NoError(t, err)

	httpCli, ok := client.httpCli.(*http.Client)
	require.True(t, ok)
	require.Equal(t, defaultMCPClientTimeout, httpCli.Timeout)
}
