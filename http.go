package utils

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

// CtxKey context key type
type CtxKey string

// String get string value of context key
func (k CtxKey) String() string {
	return string(k)
}

const (
	defaultHTTPClientOptTimeout    = 30 * time.Second
	defaultHTTPClientOptMaxConn    = 20
	maxRequestJSONSuccessBodyBytes = 8 * 1024 * 1024
	maxRequestJSONErrorBodyBytes   = 8 * 1024

	// HTTPHeaderHost HTTP header name
	HTTPHeaderHost = "Host"
	// HTTPHeaderReferer HTTP header name
	HTTPHeaderReferer = "Referer"
	// HTTPHeaderContentType HTTP header name
	HTTPHeaderContentType = "Content-Type"

	// HTTPHeaderContentTypeValJSON HTTP header value
	HTTPHeaderContentTypeValJSON = "application/json"

	// TracingKey default trace key
	//
	// https://www.jaegertracing.io/docs/1.22/client-libraries/#key
	//
	//  `{trace-id}:{span-id}:{parent-span-id}:{flags}`
	TracingKey CtxKey = "Uber-Trace-Id"
)

var (
	internalHttpCli *http.Client
)

// init builds the package-level internalHttpCli that RequestJSON and RequestJSONWithClient fall back to,
// configured with a 30-second timeout and proxy settings read from the environment. It takes no parameters,
// returns nothing, and panics through the shared logger if the HTTP client cannot be constructed.
func init() {
	var err error

	// new http client
	opts := []HTTPClientOptFunc{
		WithHTTPClientTimeout(30 * time.Second),
		WithHTTPClientProxyFromEnvironment(),
	}
	if internalHttpCli, err = NewHTTPClient(opts...); err != nil {
		log.Shared.Panic("new http client got error", zap.Error(err))
	}
}

type httpClientOption struct {
	timeout   time.Duration
	maxConn   int
	insecure  bool
	tlsConfig *tls.Config
	proxy     func(*http.Request) (*url.URL, error)
}

// HTTPClientOptFunc http client options
type HTTPClientOptFunc func(*httpClientOption) error

// WithHTTPClientTimeout set http client timeout
//
// default to 30s
func WithHTTPClientTimeout(timeout time.Duration) HTTPClientOptFunc {
	return func(opt *httpClientOption) error {
		if timeout <= 0 {
			return errors.Errorf("timeout should greater than 0")
		}

		opt.timeout = timeout
		return nil
	}
}

// WithHTTPClientMaxConn set http client max connection
//
// default to 20
func WithHTTPClientMaxConn(maxConn int) HTTPClientOptFunc {
	return func(opt *httpClientOption) error {
		if maxConn <= 0 {
			return errors.Errorf("maxConn should greater than 0")
		}

		opt.maxConn = maxConn
		return nil
	}
}

// WithHTTPClientProxy explicitly selects a trusted fixed proxy for every destination.
// This option intentionally bypasses environment discovery and NO_PROXY rules.
// Use WithHTTPClientProxyFromEnvironment for automatic environment policy.
func WithHTTPClientProxy(proxy string) HTTPClientOptFunc {
	return func(opt *httpClientOption) (err error) {
		proxy, err := url.Parse(proxy)
		if err != nil {
			return errors.Wrap(err, "cannot parse proxy")
		}

		opt.proxy = http.ProxyURL(proxy)
		return nil
	}
}

// WithHTTPClientInsecure set http client igonre ssl issue
//
// default to false
//
// Deprecated: use WithHTTPTlsConfig instead
func WithHTTPClientInsecure() HTTPClientOptFunc {
	return func(opt *httpClientOption) error {
		opt.insecure = true
		return nil
	}
}

// WithHTTPTlsConfig set tls config
func WithHTTPTlsConfig(cfg *tls.Config) HTTPClientOptFunc {
	return func(opt *httpClientOption) error {
		opt.tlsConfig = cfg
		return nil
	}
}

// NewHTTPClient creates a direct HTTP client unless a proxy option is supplied.
// The package's default RequestJSON client separately opts into environment policy.
func NewHTTPClient(opts ...HTTPClientOptFunc) (c *http.Client, err error) {
	opt := &httpClientOption{
		maxConn: defaultHTTPClientOptMaxConn,
		timeout: defaultHTTPClientOptTimeout,
	}
	for _, optf := range opts {
		if err = optf(opt); err != nil {
			return nil, errors.Wrap(err, "set option")
		}
	}

	// deprecated in 5.0
	if opt.tlsConfig == nil && opt.insecure {
		opt.tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	c = &http.Client{
		Transport: &http.Transport{
			Proxy:               opt.proxy,
			MaxIdleConnsPerHost: opt.maxConn,
			TLSClientConfig:     opt.tlsConfig,
		},
		Timeout: opt.timeout,
	}

	return c, nil
}

// NewReusableRequest creates a new HTTP request that can be reused with the same body.
// This function handles different types of body readers and ensures that the
// request can be reused, which is critical for handling HTTP/2 GOAWAY frames
// and redirects that require replaying the request body.
//
// The function automatically sets the GetBody field for reusable reader types:
//   - *bytes.Buffer: Fully reusable, GetBody is set automatically
//   - *bytes.Reader: Fully reusable, GetBody is set automatically
//   - *strings.Reader: Fully reusable, GetBody is set automatically
//   - Other io.Reader types: Request is created but may not be reusable for redirects
//
// When an HTTP/2 server sends a GOAWAY frame, the client needs to retry the request
// on a new connection. If the request has a body and GetBody is nil, the retry will
// fail. This function ensures that common reader types are properly handled.
//
// Args:
//   - ctx: Context for the request, used for cancellation and timeouts
//   - method: HTTP method (GET, POST, PUT, etc.)
//   - url: Target URL for the request
//   - body: Request body reader, nil for requests without body
//
// Returns:
//   - *http.Request: The created request with proper GetBody handling
//   - error: Error if request creation fails (invalid URL, method, etc.)
//
// Example:
//
//	// Reusable request with bytes.Buffer
//	data := bytes.NewBufferString(`{"key": "value"}`)
//	req, err := NewReusableRequest(ctx, "POST", "https://api.example.com", data)
//
//	// Non-reusable request (will work but may fail on HTTP/2 GOAWAY)
//	reader := io.NopCloser(strings.NewReader(`{"key": "value"}`))
//	req, err := NewReusableRequest(ctx, "POST", "https://api.example.com", reader)
//
// Reference: https://cs.opensource.google/go/go/+/refs/tags/go1.24.4:src/net/http/request.go;l=924
func NewReusableRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	switch body.(type) {
	case *bytes.Buffer, *bytes.Reader, *strings.Reader:
		// These types are automatically handled by http.NewRequestWithContext
		// which sets GetBody appropriately for reusability with HTTP/2 GOAWAY
		// frames and redirects that need to replay the request body
		return http.NewRequestWithContext(ctx, method, url, body)
	default:
		// For other readers, pass through directly but won't be reusable for redirects
		// or HTTP/2 GOAWAY scenarios that require body replay
		return http.NewRequestWithContext(ctx, method, url, body)
	}
}
