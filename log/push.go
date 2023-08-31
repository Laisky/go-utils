package log

import (
	"bytes"
	"context"
	"net/http"

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
	return &PusherHTTPSender{
		httpcli:        httpcli,
		remoteEndpoint: remoteEndpoint,
		headers:        headers,
	}
}

// Send send log to remote
func (s *PusherHTTPSender) Send(ctx context.Context, content []byte) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.remoteEndpoint, bytes.NewReader(content))
	if err != nil {
		return errors.Wrapf(err, "create request to %s", s.remoteEndpoint)
	}
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	resp, err := s.httpcli.Do(req)
	if err != nil {
		return errors.Wrapf(err, "send request to %s", s.remoteEndpoint)
	}
	defer func() {
		if deferErr := resp.Body.Close(); deferErr != nil {
			Shared.Error("close response body", zap.Error(deferErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("got unexpected status code %d", resp.StatusCode)
	}

	return nil
}

// Format format log to bytes
func (f *PusherJSONFormatter) Format(ent zapcore.Entry, fields []zapcore.Field) (content []byte, err error) {
	buf, err := f.encoder.EncodeEntry(ent, fields)
	if err != nil {
		return nil, errors.Wrap(err, "encode entry")
	}

	return buf.Bytes(), nil
}

type pusherOption struct {
	logger        Logger
	formatter     PusherFormatter
	sender        PusherSender
	filter        func(ent zapcore.Entry, fs []zapcore.Field) bool
	senderChanLen int
}

// PusherOption pusher option
type PusherOption func(opts *pusherOption) error

func (o *pusherOption) fillDefault() *pusherOption {
	o.logger = Shared.Named("log_pusher")
	o.formatter = NewDefaultPusherFormatter()
	o.sender = &defaultPusherSender{
		logger: o.logger.Named("sender"),
	}
	o.senderChanLen = 0

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
// default is 0, means no buffer, if you want to send log asynchronously,
// set this value to a positive number.
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

// Pusher push log to remote
type Pusher struct {
	opt        *pusherOption
	senderChan chan []byte
}

// NewPusher create new pusher
//
// pusher will run a sender goroutine in background, and send log to remote asynchronously,
// ths ctx argument will be used to control the sender goroutine.
func NewPusher(ctx context.Context, opts ...PusherOption) (p *Pusher, err error) {
	opt, err := new(pusherOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply opts")
	}

	p = &Pusher{
		opt: opt,
	}
	p.senderChan = make(chan []byte, opt.senderChanLen)

	go p.sender(ctx)
	return p, err
}

func (p *Pusher) sender(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			p.opt.logger.Debug("sender goroutine exit")
			return
		case content := <-p.senderChan:
			if err := p.opt.sender.Send(ctx, content); err != nil {
				p.opt.logger.Error("send log", zap.Error(err))
			}
		}
	}
}

// GetZapHook get hook for zap logger
func (p *Pusher) GetZapHook() func(zapcore.Entry, []zapcore.Field) (err error) {
	return func(ent zapcore.Entry, fields []zapcore.Field) (err error) {
		body, err := p.opt.formatter.Format(ent, fields)
		if err != nil {
			return errors.Wrap(err, "format log")
		}

		if len(body) == 0 {
			p.opt.logger.Debug("skip empty log")
			return nil
		}

		if p.opt.filter != nil && !p.opt.filter(ent, fields) {
			p.opt.logger.Debug("skip filtered log")
			return nil
		}

		p.senderChan <- body
		return nil
	}
}
