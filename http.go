package utils

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-chaining"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/go-utils/v6/log"
)

var allowedBrowserURLSchemes = map[string]struct{}{
	"http":   {},
	"https":  {},
	"mailto": {},
}

// CtxKey context key type
type CtxKey string

// String get string value of context key
func (k CtxKey) String() string {
	return string(k)
}

const (
	defaultHTTPClientOptTimeout = 30 * time.Second
	defaultHTTPClientOptMaxConn = 20
	maxRequestJSONSuccessBodyBytes = 8 * 1024 * 1024
	maxRequestJSONErrorBodyBytes = 8 * 1024

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

func init() {
	var err error

	// new http client
	opts := []HTTPClientOptFunc{
		WithHTTPClientTimeout(30 * time.Second),
	}
	if len(GetEnvInsensitive("HTTP_PROXY")) != 0 {
		opts = append(opts, WithHTTPClientProxy(GetEnvInsensitive("HTTP_PROXY")[0]))
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

// NewJaegerTracingID generate jaeger tracing id
//
// Args:
//   - traceID: trace id, 64bit number, will encode to hex string
//   - spanID: span id, 64bit number, will encode to hex string
//   - parentSpanID: parent span id, 64bit number, will encode to hex string
//   - flag: 8bit number, one byte bitmap, as one or two hex digits (leading zero may be omitted)
//
// Even if some of the parameters have incorrect formatting,
// it won't result in an error; instead, it will generate a new random value.
func NewJaegerTracingID(traceID, spanID, parentSpanID uint64, flag byte) (traceVal JaegerTracingID, err error) {
	if traceID == 0 {
		if traceID, err = RandomNonZeroUint64(); err != nil {
			return "", errors.Wrapf(err, "generate random trace id")
		}
	}
	if spanID == 0 {
		if spanID, err = RandomNonZeroUint64(); err != nil {
			return "", errors.Wrapf(err, "generate random span id")
		}
	}
	if flag == 0 {
		flag = 0x04 // default to not used
	}

	traceIDVal := strings.TrimLeft(fmt.Sprintf("%016x", traceID), "0")
	spanIDVal := strings.TrimLeft(fmt.Sprintf("%016x", spanID), "0")
	parentSpanIDVal := strings.TrimLeft(fmt.Sprintf("%016x", parentSpanID), "0")
	flagVal := strings.TrimLeft(fmt.Sprintf("%02x", flag), "0")

	return JaegerTracingID(fmt.Sprintf("%s:%s:%s:%s", traceIDVal, spanIDVal, parentSpanIDVal, flagVal)), nil
}

// PaddingLeft padding string to left
func PaddingLeft(s string, padStr string, pLen int) string {
	if len(s) >= pLen {
		return s
	}

	return strings.Repeat(padStr, pLen-len(s)) + s
}

// JaegerTracingID jaeger tracing id
type JaegerTracingID string

// String implement fmt.Stringer
func (t JaegerTracingID) String() string {
	return string(t)
}

// Parse parse jaeger tracing id from string
func (t JaegerTracingID) Parse() (traceID, spanID, parentSpanID uint64, flag byte, err error) {
	traceVal := t.String()
	vals := strings.Split(traceVal, ":")
	if len(vals) != 4 {
		return 0, 0, 0, 0, errors.Errorf("invalid trace value `%s`", traceVal)
	}

	if traceID, err = strconv.ParseUint(PaddingLeft(vals[0], "0", 16), 16, 64); err != nil {
		return 0, 0, 0, 0, errors.Wrapf(err, "parse traceID")
	}
	if spanID, err = strconv.ParseUint(PaddingLeft(vals[1], "0", 16), 16, 64); err != nil {
		return 0, 0, 0, 0, errors.Wrapf(err, "parse spanID")
	}
	if parentSpanID, err = strconv.ParseUint(PaddingLeft(vals[2], "0", 16), 16, 64); err != nil {
		return 0, 0, 0, 0, errors.Wrapf(err, "parse parentSpanID")
	}
	if flagSlice, err := hex.DecodeString(PaddingLeft(vals[3], "0", 2)); err != nil {
		return 0, 0, 0, 0, errors.Wrapf(err, "parse flag")
	} else if len(flagSlice) != 1 {
		return 0, 0, 0, 0, errors.Errorf("invalid flag `%s`", vals[3])
	} else {
		flag = flagSlice[0]
	}

	return traceID, spanID, parentSpanID, flag, nil
}

// RandomNonZeroUint64 generate random uint64 number
func RandomNonZeroUint64() (uint64, error) {
	var num uint64
	for {
		if err := binary.Read(rand.Reader, binary.BigEndian, &num); err != nil {
			return 0, errors.Wrap(err, "generate random number")
		}

		if num != 0 {
			return num, nil
		}
	}
}

// NewSpan generate new span
func (t JaegerTracingID) NewSpan() (JaegerTracingID, error) {
	traceID, spanID, _, flag, err := t.Parse()
	if err != nil {
		return "", errors.Wrapf(err, "parse traceID")
	}

	newSpanID, err := RandomNonZeroUint64()
	if err != nil {
		return "", errors.Wrapf(err, "generate new spanID")
	}

	return NewJaegerTracingID(traceID, newSpanID, spanID, flag)
}

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

// WithHTTPClientProxy set http client proxy
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

// NewHTTPClient create http client
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

// RequestData http request
type RequestData struct {
	Headers map[string]string
	Data    any
}

type requestJSONOption struct {
	maxRespBodyBytes int64
}

type checkRespOption struct {
	maxErrBodyBytes int64
}

// RequestJSONOptFunc customizes RequestJSON and RequestJSONWithClient behavior.
type RequestJSONOptFunc func(*requestJSONOption) error

// WithRequestJSONMaxResponseBodyBytes sets the max bytes allowed for successful JSON responses.
//
// Args:
//   - maxBytes: Maximum allowed response bytes for HTTP 2xx JSON bodies.
//
// Returns:
//   - RequestJSONOptFunc: Option function.
func WithRequestJSONMaxResponseBodyBytes(maxBytes int64) RequestJSONOptFunc {
	return func(opt *requestJSONOption) error {
		if maxBytes <= 0 {
			return errors.Errorf("max response body bytes should greater than 0")
		}

		opt.maxRespBodyBytes = maxBytes
		return nil
	}
}

// newRequestJSONOption builds request JSON options from variadic opt funcs.
//
// Args:
//   - opts: Option functions.
//
// Returns:
//   - *requestJSONOption: Finalized option values.
//   - error: Option validation errors.
func newRequestJSONOption(opts ...RequestJSONOptFunc) (*requestJSONOption, error) {
	opt := &requestJSONOption{
		maxRespBodyBytes: maxRequestJSONSuccessBodyBytes,
	}

	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return nil, errors.Wrap(err, "set request option")
		}
	}

	return opt, nil
}

// CheckRespOptFunc customizes CheckResp behavior.
type CheckRespOptFunc func(*checkRespOption) error

// WithCheckRespMaxErrorBodyBytes sets the max bytes captured from non-2xx response bodies.
//
// Args:
//   - maxBytes: Maximum bytes from error response body.
//
// Returns:
//   - CheckRespOptFunc: Option function.
func WithCheckRespMaxErrorBodyBytes(maxBytes int64) CheckRespOptFunc {
	return func(opt *checkRespOption) error {
		if maxBytes <= 0 {
			return errors.Errorf("max check response error body bytes should greater than 0")
		}

		opt.maxErrBodyBytes = maxBytes
		return nil
	}
}

// newCheckRespOption builds check response options from variadic opt funcs.
//
// Args:
//   - opts: Option functions.
//
// Returns:
//   - *checkRespOption: Finalized option values.
//   - error: Option validation errors.
func newCheckRespOption(opts ...CheckRespOptFunc) (*checkRespOption, error) {
	opt := &checkRespOption{
		maxErrBodyBytes: maxRequestJSONErrorBodyBytes,
	}

	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return nil, errors.Wrap(err, "set check response option")
		}
	}

	return opt, nil
}

// RequestJSON request JSON and return JSON by default client
func RequestJSON(method, url string, request *RequestData, resp any, opts ...RequestJSONOptFunc) (err error) {
	return RequestJSONWithClient(internalHttpCli, method, url, request, resp, opts...)
}

// RequestJSONWithClient request JSON and return JSON with specific client
func RequestJSONWithClient(httpClient *http.Client,
	method,
	url string,
	request *RequestData,
	resp any,
	opts ...RequestJSONOptFunc,
) (err error) {
	if httpClient == nil {
		httpClient = internalHttpCli
	}

	requestOpt, err := newRequestJSONOption(opts...)
	if err != nil {
		return errors.Wrap(err, "new request options")
	}

	log.Shared.Debug("try to request with json", zap.String("method", method), zap.String("url", url))

	var (
		jsonBytes []byte
		body      io.Reader
	)

	if request != nil {
		jsonBytes, err = json.Marshal(request.Data)
		if err != nil {
			return errors.Wrap(err, "marshal request data error")
		}
		log.Shared.Debug("request json", zap.String("body", string(jsonBytes)))
		body = bytes.NewReader(jsonBytes)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx,
		strings.ToUpper(method), url, body)
	if err != nil {
		return errors.Wrap(err, "new request")
	}

	if request != nil {
		req.Header.Set(HTTPHeaderContentType, HTTPHeaderContentTypeValJSON)
		for k, v := range request.Headers {
			req.Header.Set(k, v)
		}
	}

	r, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "try to request url error")
	}
	defer func() { _ = r.Body.Close() }()

	if r.StatusCode/100 != 2 { //nolint:usestdlibvars //"100" can be replaced by http.StatusContinue
		respBytes, truncated, err := readHTTPBodyWithLimit(r.Body, maxRequestJSONErrorBodyBytes)
		if err != nil {
			return errors.Wrap(err, "try to read response data error")
		}

		if truncated {
			log.Shared.Debug("http error response body truncated",
				zap.Int("status_code", r.StatusCode),
				zap.Int("max_body_bytes", maxRequestJSONErrorBodyBytes),
				zap.Int("body_bytes", len(respBytes)),
			)

			return errors.New(string(respBytes[:]) + " (truncated)")
		}

		return errors.New(string(respBytes[:]))
	}

	if err = decodeJSONBodyWithLimit(r.Body, requestOpt.maxRespBodyBytes, resp); err != nil {
		log.Shared.Debug("unmarshal response failed",
			zap.Int("status_code", r.StatusCode),
			zap.Int64("max_body_bytes", requestOpt.maxRespBodyBytes),
			zap.Error(err),
		)

		return errors.Wrapf(err, "unmarshal response")
	}

	return nil
}

// decodeJSONBodyWithLimit decodes one JSON value from body with an upper memory bound.
//
// Args:
//   - body: HTTP response body reader.
//   - maxBytes: Maximum allowed body size.
//   - resp: Destination object for decoded JSON.
//
// Returns:
//   - error: Decode error or body-too-large error.
func decodeJSONBodyWithLimit(body io.Reader, maxBytes int64, resp any) error {
	respB, truncated, err := readHTTPBodyWithLimit(body, maxBytes)
	if err != nil {
		return errors.Wrap(err, "read response body")
	}

	if truncated {
		return errors.Errorf("response body too large, exceeds %d bytes", maxBytes)
	}

	if err = json.NewDecoder(bytes.NewReader(respB)).Decode(resp); err != nil {
		return errors.Wrap(err, "decode response json")
	}

	return nil
}

// readHTTPBodyWithLimit reads body up to maxBytes and reports whether truncation happened.
//
// Args:
//   - body: HTTP response body reader.
//   - maxBytes: Maximum bytes to keep in memory.
//
// Returns:
//   - []byte: Response bytes, capped at maxBytes.
//   - bool: Whether body exceeded the limit.
//   - error: Read failure.
func readHTTPBodyWithLimit(body io.Reader, maxBytes int64) ([]byte, bool, error) {
	respB, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, errors.Wrap(err, "read response body with limit")
	}

	if int64(len(respB)) > maxBytes {
		return respB[:maxBytes], true, nil
	}

	return respB, false, nil
}

// CheckResp checks HTTP response status and returns body context when status is non-2xx.
func CheckResp(resp *http.Response, opts ...CheckRespOptFunc) error {
	opt, err := newCheckRespOption(opts...)
	if err != nil {
		return errors.Wrap(err, "new check response options")
	}

	c := chaining.Flow(
		checkRespStatus,
		checkRespErr(opt.maxErrBodyBytes),
	)(resp, nil)
	return c.GetError()
}

// HTTPInvalidStatusError return error about status code
func HTTPInvalidStatusError(statusCode int) error {
	return errors.Errorf("got http invalid status code `%d`", statusCode)
}

func checkRespStatus(c *chaining.Chain) (r any, err error) {
	resp, ok := c.GetVal().(*http.Response)
	if !ok {
		return nil, errors.Errorf("got invalid response type `%T`", c.GetVal())
	}

	code := resp.StatusCode
	if code/100 != 2 {
		return resp, HTTPInvalidStatusError(code)
	}

	return resp, nil
}

func checkRespErr(maxErrBodyBytes int64) func(c *chaining.Chain) (any, error) {
	return func(c *chaining.Chain) (any, error) {
	upErr := c.GetError()
	if upErr == nil {
		return c.GetVal(), nil
	}

	resp, ok := c.GetVal().(*http.Response)
	if !ok {
		return nil, errors.Join(upErr, errors.Errorf("got invalid response type `%T`", c.GetVal()))
	}

	defer func() { _ = resp.Body.Close() }()
	respB, truncated, err := readHTTPBodyWithLimit(resp.Body, maxErrBodyBytes)
	if err != nil {
		return resp, errors.Wrapf(upErr, "read body got error: %v", err.Error())
	}

	suffix := ""
	if truncated {
		suffix = " (truncated)"
	}

	return resp, errors.Wrapf(upErr, "got http body%s: %v", suffix, string(respB))
	}
}

// OpenURLInDefaultBrowser opens the specified URL in the default browser of the user.
//
// Inspired by https://gist.github.com/sevkin/9798d67b2cb9d07cb05f89f14ba682f8?permalink_comment_id=5019685#gistcomment-5019685
//
//nolint:lll
func OpenURLInDefaultBrowser(ctx context.Context, rawURL string) error {
	parsedURL, err := validateOpenBrowserURL(rawURL)
	if err != nil {
		scheme := ""
		hasHost := false
		if parsedURL != nil {
			scheme = parsedURL.Scheme
			hasHost = parsedURL.Host != ""
		}

		log.Shared.Debug("reject url for default browser",
			zap.String("scheme", scheme),
			zap.Bool("has_host", hasHost),
			zap.Error(err),
		)

		return errors.Wrap(err, "validate url")
	}

	runningInWSL := false
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		runningInWSL = isWSL(ctx)
	}

	cmd, args := buildOpenURLCommand(runtime.GOOS, runningInWSL, rawURL)
	log.Shared.Debug("open url in default browser",
		zap.String("scheme", parsedURL.Scheme),
		zap.Bool("has_host", parsedURL.Host != ""),
		zap.String("command", cmd),
		zap.Int("args_len", len(args)),
		zap.Bool("is_wsl", runningInWSL),
	)

	//nolint:gosec //G204: Subprocess launched with variable
	return exec.CommandContext(ctx, cmd, args...).Start()
}

// validateOpenBrowserURL validates URL format and enforces scheme allowlist.
//
// Args:
//   - rawURL: Raw URL string provided by caller.
//
// Returns:
//   - *url.URL: Parsed URL.
//   - error: Validation error when URL is malformed or has disallowed scheme.
func validateOpenBrowserURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse url")
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if _, ok := allowedBrowserURLSchemes[scheme]; !ok {
		return parsedURL, errors.Errorf("unsupported url scheme `%s`", parsedURL.Scheme)
	}

	if parsedURL.Scheme == "mailto" {
		if parsedURL.Opaque == "" {
			return parsedURL, errors.Errorf("mailto url should contain recipient")
		}

		return parsedURL, nil
	}

	if parsedURL.Host == "" {
		return parsedURL, errors.Errorf("url host should not be empty")
	}

	return parsedURL, nil
}

// buildOpenURLCommand builds the OS-specific command and arguments to open a URL.
//
// Args:
//   - goos: Target operating system name.
//   - runningInWSL: Whether current Linux environment is WSL.
//   - targetURL: Validated URL to open.
//
// Returns:
//   - string: Executable command.
//   - []string: Command arguments.
func buildOpenURLCommand(goos string, runningInWSL bool, targetURL string) (string, []string) {
	switch goos {
	case "windows":
		return "cmd", []string{"/c", "start", "", targetURL}
	case "darwin":
		return "open", []string{targetURL}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		if runningInWSL {
			return "cmd.exe", []string{"/c", "start", "", targetURL}
		}

		return "xdg-open", []string{targetURL}
	}
}

// isWSL checks if the Go program is running inside Windows Subsystem for Linux
func isWSL(ctx context.Context) bool {
	releaseData, err := exec.CommandContext(ctx, "uname", "-r").Output()
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(string(releaseData)), "microsoft")
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
