package files

import (
	"context"
	"io"
	"sync"

	"github.com/Laisky/errors/v2"
)

// ErrMCPBudgetExceeded reports exhausted aggregate request or response capacity.
var ErrMCPBudgetExceeded = errors.New("MCP operation budget exceeded")

type mcpBudgetKey struct{}

// mcpBudget shares finite capacity across RPCs and retains any enclosing budget.
type mcpBudget struct {
	mu       sync.Mutex
	requests int
	bytes    int64
	parent   *mcpBudget
}

// WithMCPBudget returns a context sharing at most maxRequests HTTP attempts and
// maxResponseBytes response-body bytes across MCP clients. Both limits must be
// positive. Nested budgets also consume their parent's capacity. In-flight
// responses reserve capacity, so concurrent requests can fail admission rather
// than collectively exceeding the budget. The caller still controls deadlines.
func WithMCPBudget(ctx context.Context, maxRequests int, maxResponseBytes int64) (context.Context, error) {
	if maxRequests <= 0 || maxResponseBytes <= 0 {
		return nil, errors.New("MCP budgets must be positive")
	}
	parent, _ := ctx.Value(mcpBudgetKey{}).(*mcpBudget)
	budget := &mcpBudget{requests: maxRequests, bytes: maxResponseBytes, parent: parent}
	return context.WithValue(ctx, mcpBudgetKey{}, budget), nil
}

// reserveMCPResponse reserves one attempt and bounded response space from every
// enclosing budget. finish refunds unread reserved bytes, including on failures;
// attempts and bytes actually read are never refunded. A one-byte overflow probe
// may be consumed to distinguish an exact-size body from a truncated response.
func reserveMCPResponse(ctx context.Context, perResponse int64) (limit int64, finish func(int64), err error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, errors.Wrap(err, "MCP budget context")
	}
	var chain []*mcpBudget
	for b, _ := ctx.Value(mcpBudgetKey{}).(*mcpBudget); b != nil; b = b.parent {
		chain = append(chain, b)
	}
	for _, b := range chain {
		b.mu.Lock()
	}
	defer func() {
		for i := len(chain) - 1; i >= 0; i-- {
			chain[i].mu.Unlock()
		}
	}()
	limit = perResponse
	for _, b := range chain {
		if b.requests <= 0 || b.bytes <= 0 {
			return 0, nil, errors.WithStack(ErrMCPBudgetExceeded)
		}
		limit = min(limit, b.bytes)
	}
	for _, b := range chain {
		b.requests--
		b.bytes -= limit
	}
	reserved := limit
	var once sync.Once
	finish = func(consumed int64) {
		once.Do(func() {
			refund := max(int64(0), reserved-max(int64(0), consumed))
			for _, b := range chain {
				b.mu.Lock()
				b.bytes += refund
				b.mu.Unlock()
			}
		})
	}
	return limit, finish, nil
}

// mcpCountingReader accounts for bytes read even when the reader returns an error.
type mcpCountingReader struct {
	reader io.Reader
	count  int64
}

// Read reads into p and records the actual byte count before returning the error.
func (r *mcpCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	if err != nil {
		// io.Reader requires the exact EOF sentinel, not a wrapped EOF.
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}
		return n, errors.Wrap(err, "read MCP response")
	}
	return n, nil
}
