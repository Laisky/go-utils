package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewEngineRejectsInsecureEndpoint verifies that the endpoint-based MCP
// storage plugin rejects cleartext and userinfo-bearing endpoints before any
// bootstrap request carrying the API key is attempted. It is a regression for
// issue #47.
func TestNewEngineRejectsInsecureEndpoint(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{
		"http://agent.example.invalid/mcp",
		"https://user:pass@agent.example.invalid/mcp",
		"ftp://agent.example.invalid/mcp",
	} {
		_, err := NewEngine(context.Background(), Config{Endpoint: endpoint, APIKey: "SYNTHETIC_KEY"})
		require.Error(t, err, endpoint)
		require.NotContains(t, err.Error(), "pass@")
	}
}
