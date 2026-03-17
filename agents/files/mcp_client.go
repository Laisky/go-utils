package files

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

const (
	jsonrpcVersion             = "2.0"
	maxMCPRPCResponseBodyBytes = 8 * 1024 * 1024
	defaultMCPClientTimeout    = 30 * time.Second
)

// ErrorCode is the normalized MCP tool error code.
type ErrorCode string

const (
	// ErrorCodeInvalidPath indicates invalid project or path parameters.
	ErrorCodeInvalidPath ErrorCode = "INVALID_PATH"
	// ErrorCodeInvalidOffset indicates invalid numeric offset/length/depth.
	ErrorCodeInvalidOffset ErrorCode = "INVALID_OFFSET"
	// ErrorCodeNotFound indicates path not found.
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrorCodeIsDirectory indicates file operation against directory path.
	ErrorCodeIsDirectory ErrorCode = "IS_DIRECTORY"
	// ErrorCodeNotDirectory indicates parent segment is a file.
	ErrorCodeNotDirectory ErrorCode = "NOT_DIRECTORY"
	// ErrorCodeNotEmpty indicates deleting non-empty directory without recursive flag.
	ErrorCodeNotEmpty ErrorCode = "NOT_EMPTY"
	// ErrorCodePermissionDenied indicates forbidden operation.
	ErrorCodePermissionDenied ErrorCode = "PERMISSION_DENIED"
	// ErrorCodePayloadTooLarge indicates write payload too large.
	ErrorCodePayloadTooLarge ErrorCode = "PAYLOAD_TOO_LARGE"
	// ErrorCodeQuotaExceeded indicates project quota exceeded.
	ErrorCodeQuotaExceeded ErrorCode = "QUOTA_EXCEEDED"
	// ErrorCodeResourceBusy indicates concurrent lock timeout and can be retried.
	ErrorCodeResourceBusy ErrorCode = "RESOURCE_BUSY"
	// ErrorCodeSearchBackendError indicates search backend unavailable.
	ErrorCodeSearchBackendError ErrorCode = "SEARCH_BACKEND_ERROR"
)

// ToolError stores normalized MCP tool-level error payload.
type ToolError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}

// Error formats a tool error as a plain message.
func (err *ToolError) Error() string {
	if err == nil {
		return ""
	}

	if err.Code == "" {
		return err.Message
	}

	return string(err.Code) + ": " + err.Message
}

// IsRetryable reports whether current tool error can be retried.
func (err *ToolError) IsRetryable() bool {
	if err == nil {
		return false
	}

	if err.Retryable {
		return true
	}

	return err.Code == ErrorCodeResourceBusy
}

// ToolCaller defines a generic MCP tools/call invoker.
type ToolCaller interface {
	CallTool(ctx context.Context, toolName string, args any, out any) error
}

// HTTPDoer abstracts http.Client for testability.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// MCPClientConfig contains runtime config for MCP JSON-RPC client.
type MCPClientConfig struct {
	Endpoint string
	APIKey   string
	Client   HTTPDoer
}

// MCPClient is a minimal JSON-RPC MCP client with session caching.
type MCPClient struct {
	endpoint string
	apiKey   string
	httpCli  HTTPDoer

	mu        sync.Mutex
	sessionID string
	rpcID     int64
}

// NewMCPClient creates an MCP JSON-RPC client.
func NewMCPClient(conf MCPClientConfig) (*MCPClient, error) {
	if strings.TrimSpace(conf.Endpoint) == "" {
		return nil, errors.Errorf("endpoint is required")
	}
	if strings.TrimSpace(conf.APIKey) == "" {
		return nil, errors.Errorf("api key is required")
	}

	httpCli := conf.Client
	if httpCli == nil {
		// Use a bounded default client so misconfigured callers cannot hang forever on remote MCP calls.
		httpCli = &http.Client{Timeout: defaultMCPClientTimeout}
	}

	return &MCPClient{
		endpoint: conf.Endpoint,
		apiKey:   conf.APIKey,
		httpCli:  httpCli,
		rpcID:    1,
	}, nil
}

// CallTool calls an MCP tool and unmarshals tool JSON output into out.
func (client *MCPClient) CallTool(ctx context.Context, toolName string, args any, out any) error {
	if strings.TrimSpace(toolName) == "" {
		return errors.Errorf("tool name is required")
	}

	if err := client.ensureSession(ctx); err != nil {
		return errors.Wrap(err, "ensure session")
	}

	client.mu.Lock()
	rpcID := client.rpcID
	client.rpcID++
	sessionID := client.sessionID
	client.mu.Unlock()

	payload := rpcReq{
		JSONRPC: jsonrpcVersion,
		ID:      rpcID,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}

	var rpcResp rpcResp
	if err := client.doRPC(ctx, sessionID, payload, &rpcResp); err != nil {
		return errors.Wrap(err, "do tools/call")
	}

	if rpcResp.Error != nil {
		return errors.Wrap(client.toToolError(rpcResp.Error), "rpc error")
	}

	if rpcResp.Result.IsError {
		return errors.Wrap(client.extractToolError(rpcResp.Result.Content), "tool error")
	}

	resultText, err := client.extractResultText(rpcResp.Result)
	if err != nil {
		return errors.Wrap(err, "extract result content")
	}

	if out == nil {
		return nil
	}

	if err = json.Unmarshal([]byte(resultText), out); err != nil {
		return errors.Wrapf(err, "unmarshal tool result `%s`", toolName)
	}

	return nil
}

// ensureSession initializes MCP session and caches session id.
func (client *MCPClient) ensureSession(ctx context.Context) error {
	client.mu.Lock()
	if client.sessionID != "" {
		client.mu.Unlock()
		return nil
	}
	rpcID := client.rpcID
	client.rpcID++
	client.mu.Unlock()

	payload := rpcReq{
		JSONRPC: jsonrpcVersion,
		ID:      rpcID,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "go-utils-agents-files",
				"version": "1.0.0",
			},
		},
	}

	var rpcResp rpcResp
	if err := client.doRPC(ctx, "", payload, &rpcResp); err != nil {
		return errors.Wrap(err, "initialize session")
	}
	if rpcResp.Error != nil {
		return errors.Wrap(client.toToolError(rpcResp.Error), "initialize rpc error")
	}

	sid := strings.TrimSpace(rpcResp.SessionID)
	if sid == "" {
		return errors.Errorf("empty session id in initialize response")
	}

	client.mu.Lock()
	if client.sessionID == "" {
		client.sessionID = sid
	}
	client.mu.Unlock()

	return nil
}

// doRPC sends one JSON-RPC request and decodes JSON response body.
func (client *MCPClient) doRPC(ctx context.Context, sessionID string, reqBody rpcReq, out *rpcResp) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return errors.Wrap(err, "marshal rpc request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.Wrap(err, "new http request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+client.apiKey)
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	httpResp, err := client.httpCli.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "do http request")
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	respBody, truncated, err := readMCPBodyWithLimit(httpResp.Body, maxMCPRPCResponseBodyBytes)
	if err != nil {
		return errors.Wrap(err, "read response body")
	}
	if truncated {
		log.Shared.Debug("mcp rpc response body truncated",
			zap.Int("http_status", httpResp.StatusCode),
			zap.Int("max_body_bytes", maxMCPRPCResponseBodyBytes),
			zap.Int("body_bytes", len(respBody)),
		)

		return errors.Errorf("rpc response body exceeds limit %d bytes (truncated)", maxMCPRPCResponseBodyBytes)
	}

	if err = json.Unmarshal(respBody, out); err != nil {
		return errors.Wrap(err, "unmarshal rpc response")
	}

	if sid := strings.TrimSpace(httpResp.Header.Get("Mcp-Session-Id")); sid != "" {
		out.SessionID = sid
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		if out.Error != nil {
			return errors.Wrap(client.toToolError(out.Error), "rpc http status error")
		}

		return errors.Errorf("rpc http status=%d", httpResp.StatusCode)
	}

	return nil
}

// readMCPBodyWithLimit reads body up to maxBytes and reports truncation.
//
// Args:
//   - body: HTTP response body reader.
//   - maxBytes: Maximum bytes to keep in memory.
//
// Returns:
//   - []byte: Response bytes, capped at maxBytes.
//   - bool: Whether body exceeded limit.
//   - error: Read failure.
func readMCPBodyWithLimit(body io.Reader, maxBytes int64) ([]byte, bool, error) {
	respB, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, errors.Wrap(err, "read mcp response body with limit")
	}

	if int64(len(respB)) > maxBytes {
		return respB[:maxBytes], true, nil
	}

	return respB, false, nil
}

// extractResultText converts MCP content payload into a JSON string.
func (client *MCPClient) extractResultText(result rpcResult) (string, error) {
	for _, item := range result.Content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text, nil
		}
	}

	if len(result.StructuredContent) != 0 {
		buf, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return "", errors.Wrap(err, "marshal structured content")
		}

		return string(buf), nil
	}

	return "", errors.Errorf("empty tool response content")
}

// extractToolError converts tool content payload into ToolError.
func (client *MCPClient) extractToolError(content []rpcContent) error {
	for _, item := range content {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}

		var tErr ToolError
		if err := json.Unmarshal([]byte(item.Text), &tErr); err == nil && (tErr.Code != "" || tErr.Message != "") {
			return &tErr
		}

		return &ToolError{Message: item.Text}
	}

	return &ToolError{Message: "unknown MCP tool error"}
}

// toToolError normalizes JSON-RPC error object to ToolError.
func (client *MCPClient) toToolError(rpcErr *rpcError) error {
	if rpcErr == nil {
		return &ToolError{Message: "unknown rpc error"}
	}

	if len(rpcErr.Data) == 0 {
		return &ToolError{Message: rpcErr.Message}
	}

	var tErr ToolError
	if err := json.Unmarshal(rpcErr.Data, &tErr); err == nil && (tErr.Code != "" || tErr.Message != "") {
		if tErr.Message == "" {
			tErr.Message = rpcErr.Message
		}

		return &tErr
	}

	return &ToolError{Message: rpcErr.Message}
}

type rpcReq struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int64          `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC   string    `json:"jsonrpc"`
	ID        int64     `json:"id"`
	Result    rpcResult `json:"result"`
	Error     *rpcError `json:"error,omitempty"`
	SessionID string    `json:"-"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcResult struct {
	IsError           bool           `json:"isError,omitempty"`
	Content           []rpcContent   `json:"content,omitempty"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
}

type rpcContent struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}
