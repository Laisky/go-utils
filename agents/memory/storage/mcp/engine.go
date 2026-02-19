package mcp

import (
	"context"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/agents/files"
	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// Config controls MCP-backed memory storage engine initialization.
type Config struct {
	Caller       files.ToolCaller
	Endpoint     string
	APIKey       string
	RetryDelays  []time.Duration
	DefaultDepth int
	DefaultLimit int
}

// NewEngine creates an MCP plugin implementing the standard memory storage interface.
//
// Parameters:
//   - ctx: The context used for endpoint-based bootstrap checks.
//   - conf: The MCP plugin configuration.
//
// Returns:
//   - memorystorage.Engine: The initialized MCP-backed storage engine.
//   - error: Non-nil when configuration is invalid or MCP bootstrap fails.
func NewEngine(ctx context.Context, conf Config) (memorystorage.Engine, error) {
	if conf.Caller != nil {
		engine, err := files.NewMCPStorage(files.MCPStorageConfig{
			Caller:       conf.Caller,
			RetryDelays:  conf.RetryDelays,
			DefaultDepth: conf.DefaultDepth,
			DefaultLimit: conf.DefaultLimit,
		})
		if err != nil {
			return nil, errors.Wrap(err, "new mcp storage from caller")
		}

		return engine, nil
	}

	if strings.TrimSpace(conf.Endpoint) == "" {
		return nil, errors.Errorf("endpoint is required when caller is nil")
	}
	if strings.TrimSpace(conf.APIKey) == "" {
		return nil, errors.Errorf("api key is required when caller is nil")
	}

	engine, err := files.NewMCPStorageFromConfig(ctx, conf.Endpoint, conf.APIKey)
	if err != nil {
		return nil, errors.Wrap(err, "new mcp storage from config")
	}

	return engine, nil
}
