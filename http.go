package utils

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/go-chaining"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

const (
	defaultHTTPClientOptTimeout  = 30 * time.Second
	defaultHTTPClientOptMaxConn  = 20
	defaultHTTPClientOptInsecure = false
)

var (
	httpClient, _         = GetHTTPClient()
	httpClientInsecure, _ = GetHTTPClient(WithHTTPClientInsecure(true))
)

type httpClientOption struct {
	timeout  time.Duration
	maxConn  int
	insecure bool
}

// HttpClientOptFunc http client options
type HttpClientOptFunc func(*httpClientOption) error

// WithHTTPClientTimeout set http client timeout
func WithHTTPClientTimeout(timeout time.Duration) HttpClientOptFunc {
	return func(opt *httpClientOption) error {
		if timeout <= 0 {
			return fmt.Errorf("timeout should greater than 0")
		}

		opt.timeout = timeout
		return nil
	}
}

// WithHTTPClientMaxConn set http client max connection
func WithHTTPClientMaxConn(maxConn int) HttpClientOptFunc {
	return func(opt *httpClientOption) error {
		if maxConn <= 0 {
			return fmt.Errorf("maxConn should greater than 0")
		}

		opt.maxConn = maxConn
		return nil
	}
}

// WithHTTPClientInsecure set http client igonre ssl issue
func WithHTTPClientInsecure(insecure bool) HttpClientOptFunc {
	return func(opt *httpClientOption) error {
		opt.insecure = insecure
		return nil
	}
}

// GetHTTPClient get default http client
func GetHTTPClient(opts ...HttpClientOptFunc) (c *http.Client, err error) {
	opt := &httpClientOption{
		maxConn:  defaultHTTPClientOptMaxConn,
		timeout:  defaultHTTPClientOptTimeout,
		insecure: defaultHTTPClientOptInsecure,
	}
	for _, optf := range opts {
		if err = optf(opt); err != nil {
			return errors.Wrap(err, "set option")
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

	return
}

// RequestData 发起请求的结构体
type RequestData struct {
	Headers map[string]string
	Data    interface{}
}

// RequestJSON request JSON and return JSON by default client
func RequestJSON(method, url string, request *RequestData, resp interface{}) (err error) {
	return RequestJSONWithClient(httpClient, method, url, request, resp)
}

// RequestJSONWithClient request JSON and return JSON with specific client
func RequestJSONWithClient(httpClient *http.Client, method, url string, request *RequestData, resp interface{}) (err error) {
	Logger.Debug("try to request with json", zap.String("method", method), zap.String("url", url))

	var (
		jsonBytes []byte
	)
	jsonBytes, err = json.Marshal(request.Data)
	if err != nil {
		return errors.Wrap(err, "marshal request data error")
	}
	Logger.Debug("request json", zap.String("body", string(jsonBytes[:])))

	req, err := http.NewRequest(strings.ToUpper(method), url, bytes.NewBuffer(jsonBytes))
	req.Header.Set(HTTPJSONHeader, HTTPJSONHeaderVal)
	for k, v := range request.Headers {
		req.Header.Set(k, v)
	}

	r, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "try to request url error")
	}
	defer r.Body.Close()

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
	Logger.Debug("got resp", zap.ByteString("resp", respBytes))
	err = json.Unmarshal(respBytes, resp)
	if err != nil {
		errMsg := fmt.Sprintf("try to unmarshal response data error: %v\n%v", err, string(respBytes[:]))
		return errors.Wrap(err, errMsg)
	}
	Logger.Debug("request json successed", zap.String("body", string(respBytes[:])))

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

	defer resp.Body.Close()
	respB, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp, errors.Wrapf(upErr, "read body got error: %v", err.Error())
	}

	return resp, errors.Wrapf(upErr, "got http body: %v", string(respB[:]))
}
