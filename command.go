package utils

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

const (
	// DefaultCMDOutputBytes is the combined stdout/stderr budget for command helpers.
	DefaultCMDOutputBytes = 1024 * 1024
	// DefaultCMDLineBytes is the streaming line limit, excluding newline.
	DefaultCMDLineBytes = 64 * 1024
	// DefaultCMDWaitDelay bounds inherited-pipe cleanup after exit or cancellation.
	DefaultCMDWaitDelay = 2 * time.Second
)

var (
	// ErrCMDOutputLimit reports that combined stdout and stderr exceeded the budget.
	ErrCMDOutputLimit = errors.New("command output exceeds byte limit")
	// ErrCMDLineLimit reports an oversized streaming line.
	ErrCMDLineLimit = errors.New("command output exceeds line limit")
)

// CMDOptions sets finite command output and cleanup limits. Zero selects defaults;
// negative values are invalid. Larger trusted outputs require an explicit budget.
// WaitDelay does not set an execution deadline: supply one in the caller context.
type CMDOptions struct {
	MaxOutputBytes int
	MaxLineBytes   int
	WaitDelay      time.Duration
}

// normalized validates options and fills zero-valued fields with safe defaults.
func (o CMDOptions) normalized() (CMDOptions, error) {
	if o.MaxOutputBytes < 0 || o.MaxLineBytes < 0 || o.WaitDelay < 0 {
		return o, errors.New("command limits must not be negative")
	}
	if o.MaxOutputBytes == 0 {
		o.MaxOutputBytes = DefaultCMDOutputBytes
	}
	if o.MaxLineBytes == 0 {
		o.MaxLineBytes = DefaultCMDLineBytes
	}
	if o.WaitDelay == 0 {
		o.WaitDelay = DefaultCMDWaitDelay
	}
	return o, nil
}

// RunCMD captures bounded combined output using default limits.
func RunCMD(ctx context.Context, app string, args ...string) ([]byte, error) {
	return RunCMDWithEnv(ctx, app, args, nil)
}

// RunCMDWithEnv captures bounded combined output. Nonempty envs replaces the
// child's environment; empty envs inherits it, preserving the existing API.
func RunCMDWithEnv(ctx context.Context, app string, args, envs []string) ([]byte, error) {
	return RunCMDWithOptions(ctx, app, args, envs, CMDOptions{})
}

// RunCMDWithOptions captures at most MaxOutputBytes across both streams. A limit
// violation cancels and reaps the direct child. Execution errors never embed argv
// or output; the returned bytes are application data, not sanitized diagnostics.
func RunCMDWithOptions(ctx context.Context, app string, args, envs []string, opts CMDOptions) ([]byte, error) {
	cmd, state, cancel, err := prepareCMD(ctx, app, args, envs, opts)
	if err != nil {
		return nil, err
	}
	defer cancel()
	writer := &cmdCapture{state: state}
	cmd.Stdout, cmd.Stderr = writer, writer
	runErr := cmd.Run()
	return writer.buf.Bytes(), finishCMD(ctx, state, runErr)
}

// RunCMD2 streams bounded lines with default limits. Callbacks for different
// streams may run concurrently and must return promptly. All callbacks complete
// before return; a blocking callback cannot be forcibly stopped by Go.
func RunCMD2(ctx context.Context, app string, args, envs []string, stdoutHandler, stderrHandler func(string)) error {
	return RunCMD2WithOptions(ctx, app, args, envs, stdoutHandler, stderrHandler, CMDOptions{})
}

// RunCMD2WithOptions cancels on framing or byte-limit failures. Nil handlers keep
// existing debug/error logging behavior. Sensitive output needs explicit handlers.
func RunCMD2WithOptions(ctx context.Context, app string, args, envs []string,
	stdoutHandler, stderrHandler func(string), opts CMDOptions) error {
	opts, err := opts.normalized()
	if err != nil {
		return err
	}
	cmd, state, cancel, err := prepareCMD(ctx, app, args, envs, opts)
	if err != nil {
		return err
	}
	defer cancel()
	if stdoutHandler == nil {
		stdoutHandler = func(s string) { log.Shared.Debug("run cmd", zap.String("msg", s)) }
	}
	if stderrHandler == nil {
		stderrHandler = func(s string) { log.Shared.Error("run cmd", zap.String("msg", s)) }
	}
	out := &cmdLines{state: state, limit: opts.MaxLineBytes, handler: stdoutHandler}
	errOut := &cmdLines{state: state, limit: opts.MaxLineBytes, handler: stderrHandler}
	cmd.Stdout, cmd.Stderr = out, errOut
	// os/exec owns and joins the pipe-copy goroutines. StdoutPipe readers must
	// not race Wait, which closes their descriptors when the process exits.
	runErr := cmd.Run()
	if state.failure() == nil {
		out.flush()
		errOut.flush()
	}
	return finishCMD(ctx, state, runErr)
}

// prepareCMD resolves argv without shell expansion and allocates invocation-local
// cancellation and accounting before starting a process.
func prepareCMD(ctx context.Context, app string, args, envs []string,
	opts CMDOptions) (*exec.Cmd, *cmdBudget, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, nil, errors.New("command context must not be nil")
	}
	opts, err := opts.normalized()
	if err != nil {
		return nil, nil, nil, err
	}
	resolved, err := resolveExecutablePath(app)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "resolve command executable")
	}
	args, err = SanitizeCMDArgs(args)
	if err != nil {
		return nil, nil, nil, errors.New("invalid command arguments")
	}
	childCtx, cancel := context.WithCancel(ctx)
	//nolint:gosec // Resolved executable with separate arguments, not shell expansion.
	cmd := exec.CommandContext(childCtx, resolved, args...)
	if len(envs) != 0 {
		cmd.Env = append([]string(nil), envs...)
	}
	cmd.WaitDelay = opts.WaitDelay
	return cmd, &cmdBudget{remaining: opts.MaxOutputBytes, cancel: cancel}, cancel, nil
}

// finishCMD preserves the first output failure ahead of secondary kill errors.
func finishCMD(ctx context.Context, state *cmdBudget, runErr error) error {
	if err := state.failure(); err != nil {
		return errors.Wrap(err, "read command output")
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "run command")
	}
	if runErr != nil {
		return errors.Wrap(runErr, "run command")
	}
	return nil
}

// cmdBudget synchronizes combined output accounting and first-failure selection.
type cmdBudget struct {
	mu        sync.Mutex
	remaining int
	err       error
	cancel    context.CancelFunc
}

// accept reserves bytes without allowing concurrent streams to exceed the budget.
func (b *cmdBudget) accept(size int) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return 0, b.err
	}
	n := min(size, b.remaining)
	b.remaining -= n
	if n < size {
		b.err = ErrCMDOutputLimit
		b.cancel()
	}
	return n, b.err
}

// fail records a framing error once and cancels the direct child immediately.
func (b *cmdBudget) fail(err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err == nil {
		b.err = err
		b.cancel()
	}
	return b.err
}

// failure returns the synchronized first output error.
func (b *cmdBudget) failure() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

// cmdCapture retains only the accepted prefix. os/exec serializes writes when
// Stdout and Stderr reference the same comparable writer.
type cmdCapture struct {
	state *cmdBudget
	buf   bytes.Buffer
}

// Write copies at most the remaining bytes and cancels on overflow.
func (w *cmdCapture) Write(p []byte) (int, error) {
	n, err := w.state.accept(len(p))
	_, _ = w.buf.Write(p[:n]) // bytes.Buffer.Write always returns len(p), nil.
	return n, err
}

// cmdLines frames one output stream without growing beyond its line budget.
type cmdLines struct {
	state   *cmdBudget
	limit   int
	handler func(string)
	line    []byte
}

// Write accounts bytes, splits newline-delimited records, and rejects long lines.
func (w *cmdLines) Write(p []byte) (int, error) {
	n, limitErr := w.state.accept(len(p))
	for rest := p[:n]; len(rest) != 0; {
		end := bytes.IndexByte(rest, '\n')
		size := len(rest)
		if end >= 0 {
			size = end
		}
		if size > w.limit-len(w.line) {
			return 0, w.state.fail(ErrCMDLineLimit)
		}
		w.line = append(w.line, rest[:size]...)
		rest = rest[size:]
		if end < 0 {
			break
		}
		w.emit()
		rest = rest[1:]
	}
	return n, limitErr
}

// emit delivers an owned line with bufio.ScanLines-compatible CR trimming.
func (w *cmdLines) emit() {
	w.handler(string(bytes.TrimSuffix(w.line, []byte{'\r'})))
	w.line = w.line[:0]
}

// flush emits a final unterminated line after the pipe-copy goroutine has joined.
func (w *cmdLines) flush() {
	if len(w.line) != 0 {
		w.emit()
	}
}
