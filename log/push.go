package log

import (
	"bytes"
	"context"
	"github.com/Laisky/go-utils/v6/internal/netdiag"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

// PusherInterface push log to remote
type PusherInterface interface {
}

// PusherFormatter format log to bytes
type PusherFormatter interface {
	Format(ent zapcore.Entry, fields []zapcore.Field) (content []byte, err error)
}

// PusherSender send log to remote
type PusherSender interface {
	Send(ctx context.Context, content []byte) (err error)
}

// PusherJSONFormatter default formatter
type PusherJSONFormatter struct {
	encoder zapcore.Encoder
}

// NewDefaultPusherFormatter create new PusherJSONFormatter
func NewDefaultPusherFormatter() *PusherJSONFormatter {
	return &PusherJSONFormatter{
		encoder: zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}),
	}
}

type defaultPusherSender struct {
	logger Logger
}

// Send send log to remote
func (s *defaultPusherSender) Send(_ context.Context, content []byte) (err error) {
	s.logger.Info("send log to remote", zap.ByteString("content", content))
	return nil
}

// PusherHTTPSender send log to remote via http
type PusherHTTPSender struct {
	remoteEndpoint string
	headers        map[string]string
	httpcli        *http.Client
}

// NewPusherHTTPSender create new PusherHTTPSender
func NewPusherHTTPSender(
	httpcli *http.Client,
	remoteEndpoint string,
	headers map[string]string) *PusherHTTPSender {
	if httpcli == nil {
		httpcli = &http.Client{}
	}
	client := *httpcli
	return &PusherHTTPSender{
		httpcli:        &client,
		remoteEndpoint: remoteEndpoint,
		headers:        maps.Clone(headers),
	}
}

// Send send log to remote
func (s *PusherHTTPSender) Send(ctx context.Context, content []byte) (err error) {
	if ctx == nil {
		return errors.New("HTTP sender context must not be nil")
	}
	ctx, cancel := context.WithTimeout(ctx, defaultPusherSendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.remoteEndpoint, bytes.NewReader(content))
	if err != nil {
		return errors.WithStack(netdiag.New("create log request", s.remoteEndpoint, err))
	}
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	resp, err := s.httpcli.Do(req)
	if err != nil {
		return errors.WithStack(netdiag.New("send log request", s.remoteEndpoint, err))
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = errors.WithStack(netdiag.New("close log response", s.remoteEndpoint, closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("got unexpected status code %d", resp.StatusCode)
	}

	return nil
}

// Format format log to bytes
func (f *PusherJSONFormatter) Format(ent zapcore.Entry, fields []zapcore.Field) (content []byte, err error) {
	buf, err := f.encoder.Clone().EncodeEntry(ent, fields)
	if err != nil {
		return nil, errors.Wrap(err, "encode entry")
	}

	defer buf.Free()
	return bytes.Clone(buf.Bytes()), nil
}

type pusherOption struct {
	logger          Logger
	formatter       PusherFormatter
	sender          PusherSender
	filter          func(ent zapcore.Entry, fs []zapcore.Field) bool
	senderChanLen   int
	sendTimeout     time.Duration
	maxMessageBytes int
}

// PusherOption pusher option
type PusherOption func(opts *pusherOption) error

func (o *pusherOption) fillDefault() *pusherOption {
	o.logger = Shared.Named("log_pusher")
	o.formatter = NewDefaultPusherFormatter()
	o.sender = &defaultPusherSender{
		logger: o.logger.Named("sender"),
	}
	o.senderChanLen = 128
	o.sendTimeout = defaultPusherSendTimeout
	o.maxMessageBytes = 64 * 1024

	return o
}

func (o *pusherOption) applyOpts(opts ...PusherOption) (*pusherOption, error) {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, errors.Wrap(err, "apply opts")
		}
	}

	return o, nil
}

// WithPusherLogger set logger
func WithPusherLogger(logger Logger) PusherOption {
	return func(o *pusherOption) error {
		if logger == nil {
			return errors.New("logger should not be nil")
		}

		o.logger = logger
		return nil
	}
}

// WithPusherFormatter set formatter
//
// default is PusherJSONFormatter
func WithPusherFormatter(formatter PusherFormatter) PusherOption {
	return func(o *pusherOption) error {
		if formatter == nil {
			return errors.New("formatter should not be nil")
		}

		o.formatter = formatter
		return nil
	}
}

// WithPusherSender set sender
func WithPusherSender(sender PusherSender) PusherOption {
	return func(o *pusherOption) error {
		if sender == nil {
			return errors.New("sender should not be nil")
		}

		o.sender = sender
		return nil
	}
}

// WithPusherSenderChanLen set sender chan len
//
// The default is 128. Zero selects an unbuffered queue. Enqueue is always
// nonblocking: entries are rejected and counted when no receiver/space is ready.
func WithPusherSenderChanLen(senderChanLen int) PusherOption {
	return func(o *pusherOption) error {
		if senderChanLen < 0 {
			return errors.Errorf("sender chan len must be positive, got %d", senderChanLen)
		}

		o.senderChanLen = senderChanLen
		return nil
	}
}

// WithPusherFilter set filter
//
// default is nil, means no filter, if you want to filter some log, set this value.
// return true means log will be sent to remote, return false means log will be dropped.
func WithPusherFilter(filter func(ent zapcore.Entry, fs []zapcore.Field) bool) PusherOption {
	return func(o *pusherOption) error {
		if filter == nil {
			return errors.New("filter should not be nil")
		}

		o.filter = filter
		return nil
	}
}

// Pusher delivers log entries through one worker and a bounded, nonblocking queue.
// Custom senders must honor their context; callbacks cannot be forcibly stopped.
// Close cancels in-flight delivery and abandons pending entries; it does not flush.
type Pusher struct {
	// admission serializes the hook's final admission check with Close and
	// worker exit. User callbacks and network sends never hold this lock.
	admission  sync.Mutex
	opt        *pusherOption
	senderChan chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	counters   pusherCounters
}

// NewPusher starts a single sender worker tied to ctx. Nil contexts are rejected.
func NewPusher(ctx context.Context, opts ...PusherOption) (*Pusher, error) {
	if ctx == nil {
		return nil, errors.New("pusher context must not be nil")
	}
	opt, err := new(pusherOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply opts")
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &Pusher{opt: opt, ctx: ctx, cancel: cancel, done: make(chan struct{}), senderChan: make(chan []byte, opt.senderChanLen)}
	go p.sender(ctx)
	return p, nil
}

// sender serializes delivery attempts with a deadline and never logs failures
// through hooks that could point back to the same pusher. Stats exposes outcomes.
func (p *Pusher) sender(ctx context.Context) {
	defer func() {
		p.admission.Lock()
		defer p.admission.Unlock()
		close(p.done)
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case content := <-p.senderChan:
			if ctx.Err() != nil {
				return
			}
			attempt, cancel := context.WithTimeout(ctx, p.opt.sendTimeout)
			err := p.opt.sender.Send(attempt, content)
			cancel()
			if err != nil {
				p.counters.failed.Add(1)
			} else {
				p.counters.delivered.Add(1)
			}
		}
	}
}

// GetZapHook returns a nonblocking best-effort delivery hook. Filtering runs
// before formatting. Custom filters/formatters must themselves return promptly.
func (p *Pusher) GetZapHook() func(zapcore.Entry, []zapcore.Field) error {
	return func(ent zapcore.Entry, fields []zapcore.Field) error {
		if p.ctx.Err() != nil {
			p.counters.dropped.Add(1)
			return errors.WithStack(ErrPusherClosed)
		}
		if p.opt.filter != nil && !p.opt.filter(ent, fields) {
			return nil
		}
		body, err := p.opt.formatter.Format(ent, fields)
		if err != nil {
			return errors.Wrap(err, "format log")
		}
		if len(body) == 0 {
			return nil
		}
		if len(body) > p.opt.maxMessageBytes {
			p.counters.dropped.Add(1)
			return errors.WithStack(ErrPusherMessageTooLarge)
		}
		// Queue ownership is independent of a formatter's reusable backing buffer.
		body = bytes.Clone(body)
		p.admission.Lock()
		defer p.admission.Unlock()
		// The formatter may have overlapped cancellation. Once Done closes no
		// admission can still be in flight or become successful afterward.
		if p.ctx.Err() != nil {
			p.counters.dropped.Add(1)
			return errors.WithStack(ErrPusherClosed)
		}
		select {
		case p.senderChan <- body:
			p.counters.enqueued.Add(1)
			return nil
		default:
			p.counters.dropped.Add(1)
			return errors.WithStack(ErrPusherQueueFull)
		}
	}
}
