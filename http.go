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
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-chaining"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/go-utils/v4/log"
)

const (
	defaultHTTPClientOptTimeout = 30 * time.Second
	defaultHTTPClientOptMaxConn = 20

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
	TracingKey = "Uber-Trace-Id"
)

var (
	httpClient, _ = NewHTTPClient()
)

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

// RequestJSON request JSON and return JSON by default client
func RequestJSON(method, url string, request *RequestData, resp any) (err error) {
	return RequestJSONWithClient(httpClient, method, url, request, resp)
}

// RequestJSONWithClient request JSON and return JSON with specific client
func RequestJSONWithClient(httpClient *http.Client,
	method,
	url string,
	request *RequestData,
	resp any,
) (err error) {
	log.Shared.Debug("try to request with json", zap.String("method", method), zap.String("url", url))

	var (
		jsonBytes []byte
	)
	jsonBytes, err = json.Marshal(request.Data)
	if err != nil {
		return errors.Wrap(err, "marshal request data error")
	}
	log.Shared.Debug("request json", zap.String("body", string(jsonBytes[:])))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx,
		strings.ToUpper(method), url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return errors.Wrap(err, "new request")
	}

	req.Header.Set(HTTPHeaderContentType, HTTPHeaderContentTypeValJSON)
	for k, v := range request.Headers {
		req.Header.Set(k, v)
	}

	r, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "try to request url error")
	}
	defer func() { _ = r.Body.Close() }()

	if r.StatusCode/100 != 2 {
		respBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return errors.Wrap(err, "try to read response data error")
		}
		return errors.New(string(respBytes[:]))
	}

	if err = json.NewDecoder(r.Body).Decode(resp); err != nil {
		return errors.Wrapf(err, "unmarshal response")
	}

	return nil
}

// CheckResp check HTTP response's status code and return the error with body message
func CheckResp(resp *http.Response) error {
	c := chaining.Flow(
		checkRespStatus,
		checkRespErr,
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

func checkRespErr(c *chaining.Chain) (any, error) {
	upErr := c.GetError()
	if upErr == nil {
		return c.GetVal(), nil
	}

	resp, ok := c.GetVal().(*http.Response)
	if !ok {
		return nil, errors.Join(upErr, errors.Errorf("got invalid response type `%T`", c.GetVal()))
	}

	defer func() { _ = resp.Body.Close() }()
	respB, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, errors.Wrapf(upErr, "read body got error: %v", err.Error())
	}

	return resp, errors.Wrapf(upErr, "got http body: %v", string(respB[:]))
}
