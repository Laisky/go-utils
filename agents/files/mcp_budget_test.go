package files

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestMCPBudgetAggregateResponses checks exact aggregate byte limits across
// several individually small responses, including no request after exhaustion.
func TestMCPBudgetAggregateResponses(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{}}`
	transport := &recordingTransport{respond: func(req *http.Request, _ []byte) (*http.Response, error) {
		return newScriptedResponse(req, 200, nil, body), nil
	}}
	client, err := NewMCPClient(MCPClientConfig{Endpoint: syntheticMCPEndpoint, APIKey: syntheticMCPKey, Client: transport})
	require.NoError(t, err)
	ctx, err := WithMCPBudget(t.Context(), 10, int64(2*len(body)))
	require.NoError(t, err)
	for range 2 {
		require.NoError(t, client.doRPC(ctx, "", rpcReq{JSONRPC: jsonrpcVersion, ID: 1, Method: "test"}, new(rpcResp)))
	}
	require.ErrorIs(t, client.doRPC(ctx, "", rpcReq{Method: "test"}, new(rpcResp)), ErrMCPBudgetExceeded)
	require.Len(t, transport.requests(), 2)
}

// TestMCPBudgetNestedAndCancellation verifies child budgets cannot erase a
// parent's request capacity and canceled requests never reach the transport.
func TestMCPBudgetNestedAndCancellation(t *testing.T) {
	transport := &recordingTransport{respond: func(req *http.Request, _ []byte) (*http.Response, error) {
		return newScriptedResponse(req, 200, nil, `{"result":{}}`), nil
	}}
	client, err := NewMCPClient(MCPClientConfig{Endpoint: syntheticMCPEndpoint, APIKey: syntheticMCPKey, Client: transport})
	require.NoError(t, err)
	parent, err := WithMCPBudget(t.Context(), 1, 1024)
	require.NoError(t, err)
	child, err := WithMCPBudget(parent, 100, 1<<20)
	require.NoError(t, err)
	require.NoError(t, client.doRPC(child, "", rpcReq{Method: "test"}, new(rpcResp)))
	require.ErrorIs(t, client.doRPC(parent, "", rpcReq{Method: "test"}, new(rpcResp)), ErrMCPBudgetExceeded)
	require.Len(t, transport.requests(), 1)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, client.doRPC(canceled, "", rpcReq{Method: "test"}, new(rpcResp)), context.Canceled)
	require.Len(t, transport.requests(), 1)
}

// TestMCPBudgetCountsPartialErrors charges bytes even when a broken body reader
// returns them together with an error, preventing partial-response retry bypass.
func TestMCPBudgetCountsPartialErrors(t *testing.T) {
	var bodies []*budgetErrorBody
	transport := &recordingTransport{respond: func(req *http.Request, _ []byte) (*http.Response, error) {
		body := &budgetErrorBody{remaining: 12}
		bodies = append(bodies, body)
		resp := newScriptedResponse(req, 200, nil, "")
		resp.Body = body
		return resp, nil
	}}
	client, err := NewMCPClient(MCPClientConfig{Endpoint: syntheticMCPEndpoint, APIKey: syntheticMCPKey, Client: transport})
	require.NoError(t, err)
	ctx, err := WithMCPBudget(t.Context(), 10, 12)
	require.NoError(t, err)
	require.ErrorIs(t, client.doRPC(ctx, "", rpcReq{Method: "test"}, new(rpcResp)), io.ErrUnexpectedEOF)
	require.ErrorIs(t, client.doRPC(ctx, "", rpcReq{Method: "test"}, new(rpcResp)), ErrMCPBudgetExceeded)
	require.Len(t, bodies, 1)
	require.True(t, bodies[0].closed)
}

// budgetErrorBody simulates a partial network body followed by a read failure.
type budgetErrorBody struct {
	remaining int
	closed    bool
}

// Read supplies bounded data and an error in the same read.
func (b *budgetErrorBody) Read(p []byte) (int, error) {
	n := min(len(p), b.remaining)
	clear(p[:n])
	b.remaining -= n
	return n, io.ErrUnexpectedEOF
}

// Close records transport cleanup without retaining any resources.
func (b *budgetErrorBody) Close() error { b.closed = true; return nil }

// TestMCPBudgetOverflowAndValidation checks bounded overflow detection and input
// rejection; the complete overlarge body is never retained or decoded.
func TestMCPBudgetOverflowAndValidation(t *testing.T) {
	_, err := WithMCPBudget(t.Context(), 0, 1)
	require.Error(t, err)
	_, err = WithMCPBudget(t.Context(), 1, 0)
	require.Error(t, err)
	transport := &recordingTransport{respond: func(req *http.Request, _ []byte) (*http.Response, error) {
		return newScriptedResponse(req, 200, nil, strings.Repeat("x", 1024)), nil
	}}
	client, err := NewMCPClient(MCPClientConfig{Endpoint: syntheticMCPEndpoint, APIKey: syntheticMCPKey, Client: transport})
	require.NoError(t, err)
	ctx, err := WithMCPBudget(t.Context(), 10, 32)
	require.NoError(t, err)
	require.ErrorIs(t, client.doRPC(ctx, "", rpcReq{Method: "test"}, new(rpcResp)), ErrMCPBudgetExceeded)
	require.Len(t, transport.requests(), 1)
	require.ErrorIs(t, client.doRPC(ctx, "", rpcReq{Method: "test"}, new(rpcResp)), ErrMCPBudgetExceeded)
	require.Len(t, transport.requests(), 1)
	// Budget errors are not retryable remote-tool failures.
	var toolErr *ToolError
	require.False(t, errors.As(ErrMCPBudgetExceeded, &toolErr))
}

// TestMCPCountingReaderContract preserves io.Reader's exact EOF sentinel and
// counts partial data while wrapping non-EOF errors with actionable context.
func TestMCPCountingReaderContract(t *testing.T) {
	reader := &mcpCountingReader{reader: strings.NewReader("fixture")}
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "fixture", string(body))
	require.Equal(t, int64(len(body)), reader.count)
	_, err = reader.Read(make([]byte, 1))
	require.Equal(t, io.EOF, err)

	reader = &mcpCountingReader{reader: &budgetErrorBody{remaining: 3}}
	n, err := reader.Read(make([]byte, 10))
	require.Equal(t, 3, n)
	require.Equal(t, int64(3), reader.count)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.ErrorContains(t, err, "read MCP response")
}
