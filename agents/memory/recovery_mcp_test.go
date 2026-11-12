package memory

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// recoveryWriteCaller applies real mock-backend writes and can lose one raw
// append acknowledgement. Only the transport boundary is fake: MCPStorage.Write
// and its retry loop are the shipped implementation.
type recoveryWriteCaller struct {
	backend     storageengine.Engine
	loseAck     bool
	failBefore  bool
	failed      bool
	rawAttempts int
	rawApplied  int
}

// CallTool forwards a file_write and injects a retryable failure before or after
// the first raw append, without calling AfterTurn a second time.
func (c *recoveryWriteCaller) CallTool(ctx context.Context, name string, args, _ any) error {
	if name != "file_write" {
		return errors.Errorf("unexpected tool %s", name)
	}
	a, ok := args.(map[string]any)
	if !ok {
		return errors.New("unexpected write arguments")
	}
	project, p, body := a["project"].(string), a["path"].(string), a["content"].(string)
	mode := storageengine.WriteMode(a["mode"].(string))
	raw := p == rawLogShardPath(recoverySession, recoveryTime) && mode == storageengine.WriteModeAppend
	if raw {
		c.rawAttempts++
		if c.failBefore && !c.failed {
			c.failed = true
			return &files.ToolError{Code: files.ErrorCodeResourceBusy, Message: "injected failure before append", Retryable: true}
		}
	}
	if err := c.backend.Write(ctx, project, p, body, mode, a["offset"].(int64)); err != nil {
		return err
	}
	if raw {
		c.rawApplied++
		if c.loseAck && !c.failed {
			c.failed = true
			return &files.ToolError{Code: files.ErrorCodeResourceBusy, Message: "injected lost append acknowledgement", Retryable: true}
		}
	}
	return nil
}

// recoveryMCPWrites uses real MCP write/retry behavior with ordinary mock reads.
type recoveryMCPWrites struct {
	storageengine.Engine
	writer *files.MCPStorage
}

// Write delegates to the production MCP adapter rather than simulating retries.
func (s recoveryMCPWrites) Write(ctx context.Context, project, p, body string, mode storageengine.WriteMode, offset int64) error {
	return s.writer.Write(ctx, project, p, body, mode, offset)
}

// newRecoveryMCPEngine returns an engine and transport fault controller backed
// by isolated synthetic data; it makes no network calls.
func newRecoveryMCPEngine(t *testing.T, loseAck, failBefore bool) (*StandardEngine, *recoveryWriteCaller) {
	t.Helper()
	backend := newMemoryStorageMock()
	caller := &recoveryWriteCaller{backend: backend, loseAck: loseAck, failBefore: failBefore}
	writer, err := files.NewMCPStorage(files.MCPStorageConfig{Caller: caller, RetryDelays: []time.Duration{0}})
	require.NoError(t, err)
	engine, err := NewEngine(recoveryMCPWrites{Engine: backend, writer: writer}, recoveryConfig())
	require.NoError(t, err)
	return engine, caller
}

// TestRecoveryMCPControls excludes duplicate input and ordinary retry handling
// as causes: no-fault and failure-before-append both persist exact state once.
func TestRecoveryMCPControls(t *testing.T) {
	for _, failBefore := range []bool{false, true} {
		name := "no_fault"
		if failBefore {
			name = "failure_before_append"
		}
		t.Run(name, func(t *testing.T) {
			engine, caller := newRecoveryMCPEngine(t, false, failBefore)
			require.NoError(t, engine.AfterTurn(context.Background(), recoveryInput("one")))
			expectedAttempts := 1
			if failBefore {
				expectedAttempts = 2
			}
			require.Equal(t, expectedAttempts, caller.rawAttempts)
			require.Equal(t, 1, caller.rawApplied)
			require.Empty(t, recoveryStateProblems(t, engine, []string{"one"}))
		})
	}
}
