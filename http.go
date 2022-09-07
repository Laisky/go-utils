package utils

import (
	"bytes"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/go-chaining"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"

	"github.com/Laisky/go-utils/v2/log"
)

const (
	defaultHTTPClientOptTimeout  = 30 * time.Second
	defaultHTTPClientOptMaxConn  = 20
	defaultHTTPClientOptInsecure = false

	// HTTPHeaderHost HTTP header name
	HTTPHeaderHost = "Host"
	// HTTPHeaderReferer HTTP header name
	HTTPHeaderReferer = "Referer"
	// HTTPHeaderContentType HTTP header name
	HTTPHeaderContentType = "Content-Type"

	// HTTPHeaderContentTypeValJSON HTTP header value
	HTTPHeaderContentTypeValJSON = "application/json"
)

var (
	httpClient, _ = NewHTTPClient()
	// httpClientInsecure, _ = NewHTTPClient(WithHTTPClientInsecure(true))
)

type httpClientOption struct {
	timeout  time.Duration
	maxConn  int
	insecure bool
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

// WithHTTPClientInsecure set http client igonre ssl issue
//
// default to false
func WithHTTPClientInsecure() HTTPClientOptFunc {
	return func(opt *httpClientOption) error {
		opt.insecure = true
		return nil
	}
}

// NewHTTPClient create http client
func NewHTTPClient(opts ...HTTPClientOptFunc) (c *http.Client, err error) {
	opt := &httpClientOption{
		maxConn:  defaultHTTPClientOptMaxConn,
		timeout:  defaultHTTPClientOptTimeout,
		insecure: defaultHTTPClientOptInsecure,
	}
	for _, optf := range opts {
		if err = optf(opt); err != nil {
			return nil, errors.Wrap(err, "set option")
		}
	}

	c = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: opt.maxConn,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: opt.insecure,
			},
		},
		Timeout: opt.timeout,
	}

	return c, nil
}

// RequestData http request
type RequestData struct {
	Headers map[string]string
	Data    interface{}
}

// RequestJSON request JSON and return JSON by default client
func RequestJSON(method, url string, request *RequestData, resp interface{}) (err error) {
	return RequestJSONWithClient(httpClient, method, url, request, resp)
}

// RequestJSONWithClient request JSON and return JSON with specific client
func RequestJSONWithClient(httpClient *http.Client,
	method,
	url string,
	request *RequestData,
	resp interface{},
) (err error) {
	log.Shared.Debug("try to request with json", zap.String("method", method), zap.String("url", url))

	var (
		jsonBytes []byte
	)
	jsonBytes, err = JSON.Marshal(request.Data)
	if err != nil {
		return errors.Wrap(err, "marshal request data error")
	}
	log.Shared.Debug("request json", zap.String("body", string(jsonBytes[:])))

	req, err := http.NewRequest(strings.ToUpper(method), url, bytes.NewBuffer(jsonBytes))
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
		respBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return errors.Wrap(err, "try to read response data error")
		}
		return errors.New(string(respBytes[:]))
	}

	respBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "try to read response data error")
	}
	log.Shared.Debug("got resp", zap.ByteString("resp", respBytes))
	err = JSON.Unmarshal(respBytes, resp)
	if err != nil {
		return errors.Wrapf(err, "unmarshal response `%s`", string(respBytes[:]))
	}
	log.Shared.Debug("request json successed", zap.String("body", string(respBytes[:])))

	return nil
}

// CheckResp check HTTP response's status code and return the error with body message
func CheckResp(resp *http.Response) error {
	c := chaining.Flow(
		checkRespStatus,
		checkRespBody,
	)(resp, nil)
	return c.GetError()
}

// HTTPInvalidStatusError return error about status code
func HTTPInvalidStatusError(statusCode int) error {
	return errors.Errorf("got http invalid status code `%d`", statusCode)
}

func checkRespStatus(c *chaining.Chain) (r interface{}, err error) {
	resp := c.GetVal()
	code := resp.(*http.Response).StatusCode
	if code/100 != 2 {
		return resp, HTTPInvalidStatusError(code)
	}

	return resp, nil
}

func checkRespBody(c *chaining.Chain) (interface{}, error) {
	upErr := c.GetError()
	resp := c.GetVal().(*http.Response)
	if upErr == nil {
		return c.GetVal(), nil
	}

	defer func() { _ = resp.Body.Close() }()
	respB, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp, errors.Wrapf(upErr, "read body got error: %v", err.Error())
	}

	return resp, errors.Wrapf(upErr, "got http body: %v", string(respB[:]))
}
