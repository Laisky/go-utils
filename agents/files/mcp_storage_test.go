package files

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// mockToolCaller provides deterministic tool call behavior for tests.
type mockToolCaller struct {
	callCnt map[string]int
	calls   []toolCall
	handler func(toolName string, args map[string]any, out any) error
}

type toolCall struct {
	toolName string
	args     map[string]any
}

// newMockToolCaller creates a mock MCP caller.
func newMockToolCaller(handler func(toolName string, args map[string]any, out any) error) *mockToolCaller {
	return &mockToolCaller{
		callCnt: make(map[string]int),
		handler: handler,
	}
}

// CallTool records calls and forwards to handler.
func (caller *mockToolCaller) CallTool(_ context.Context, toolName string, args any, out any) error {
	caller.callCnt[toolName]++
	argMap, ok := args.(map[string]any)
	if !ok {
		return errors.Errorf("args should be map")
	}

	caller.calls = append(caller.calls, toolCall{toolName: toolName, args: argMap})
	if caller.handler == nil {
		return nil
	}

	return caller.handler(toolName, argMap, out)
}

// TestMCPStorageWrite verifies write tool payload mapping.
func TestMCPStorageWrite(t *testing.T) {
	caller := newMockToolCaller(nil)
	storage, err := NewMCPStorage(MCPStorageConfig{Caller: caller})
	require.NoError(t, err)

	err = storage.Write(context.Background(), "demo", "/a.txt", "hello", WriteModeAppend, 0)
	require.NoError(t, err)
	require.Equal(t, 1, caller.callCnt["file_write"])
	require.Equal(t, "demo", caller.calls[0].args["project"])
	require.Equal(t, "/a.txt", caller.calls[0].args["path"])
	require.Equal(t, "APPEND", caller.calls[0].args["mode"])
}

// TestMCPStorageReadRetry verifies retry behavior on RESOURCE_BUSY.
func TestMCPStorageReadRetry(t *testing.T) {
	attempt := 0
	caller := newMockToolCaller(func(_ string, _ map[string]any, out any) error {
		attempt++
		if attempt == 1 {
			return &ToolError{Code: ErrorCodeResourceBusy, Message: "busy", Retryable: true}
		}

		readOut, ok := out.(*struct {
			Content string `json:"content"`
		})
		require.True(t, ok)
		readOut.Content = "ok"
		return nil
	})

	storage, err := NewMCPStorage(MCPStorageConfig{
		Caller:      caller,
		RetryDelays: []time.Duration{time.Millisecond},
	})
	require.NoError(t, err)

	content, err := storage.Read(context.Background(), "demo", "/a.txt", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "ok", content)
	require.Equal(t, 2, caller.callCnt["file_read"])
}

// TestMCPStorageStat verifies stat response mapping.
func TestMCPStorageStat(t *testing.T) {
	caller := newMockToolCaller(func(_ string, _ map[string]any, out any) error {
		statOut, ok := out.(*struct {
			Exists    bool   `json:"exists"`
			Type      string `json:"type"`
			Size      int64  `json:"size"`
			UpdatedAt string `json:"updated_at"`
		})
		require.True(t, ok)
		statOut.Exists = true
		statOut.Type = "FILE"
		statOut.Size = 3
		statOut.UpdatedAt = "2026-02-14T00:00:00Z"
		return nil
	})

	storage, err := NewMCPStorage(MCPStorageConfig{Caller: caller})
	require.NoError(t, err)

	info, err := storage.Stat(context.Background(), "demo", "/a.txt")
	require.NoError(t, err)
	require.True(t, info.Exists)
	require.Equal(t, FileTypeFile, info.Type)
	require.Equal(t, int64(3), info.SizeBytes)
}

// TestMCPStorageValidateInput verifies MCP constraint validation.
func TestMCPStorageValidateInput(t *testing.T) {
	caller := newMockToolCaller(nil)
	storage, err := NewMCPStorage(MCPStorageConfig{Caller: caller})
	require.NoError(t, err)

	err = storage.Write(context.Background(), "", "/a.txt", "x", WriteModeAppend, 0)
	require.Error(t, err)

	err = storage.Write(context.Background(), "demo", "a.txt", "x", WriteModeAppend, 0)
	require.Error(t, err)

	err = storage.Delete(context.Background(), "demo", "", false)
	require.Error(t, err)
}

// TestValidatePathRejectsControlChars verifies validatePath rejects paths that
// contain a space, a newline, or a NUL byte while accepting a normal relative
// path. Control characters (especially NUL/CR/LF) can truncate paths in C-based
// syscalls or enable injection in downstream protocols and logs.
func TestValidatePathRejectsControlChars(t *testing.T) {
	require.Error(t, validatePath("/a b.txt", false), "space must be rejected")
	require.Error(t, validatePath("/a\nb.txt", false), "newline must be rejected")
	require.Error(t, validatePath("/a\x00b.txt", false), "NUL must be rejected")
	require.Error(t, validatePath("/a\rb.txt", false), "CR must be rejected")
	require.Error(t, validatePath("/a\x7fb.txt", false), "DEL must be rejected")

	require.NoError(t, validatePath("/dir/file.txt", false), "normal relative path must be accepted")
}
