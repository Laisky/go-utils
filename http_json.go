package utils

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-chaining"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/internal/netdiag"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/go-utils/v6/log"
)

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

	log.Shared.Debug("try to request with json",
		zap.String("method", method),
		zap.String("endpoint", netdiag.Endpoint(url)))

	var (
		jsonBytes []byte
		body      io.Reader
	)

	if request != nil {
		jsonBytes, err = json.Marshal(request.Data)
		if err != nil {
			return errors.Wrap(err, "marshal request data error")
		}
		// Only log truncated body length to avoid leaking sensitive data
		// (e.g. credentials, tokens, PII) into log output.
		log.Shared.Debug("request json", zap.Int("body_bytes", len(jsonBytes)))
		body = bytes.NewReader(jsonBytes)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx,
		strings.ToUpper(method), url, body)
	if err != nil {
		return errors.WithStack(netdiag.New("create JSON request", url, err))
	}

	if request != nil {
		req.Header.Set(HTTPHeaderContentType, HTTPHeaderContentTypeValJSON)
		for k, v := range request.Headers {
			req.Header.Set(k, v)
		}
	}

	r, err := httpClient.Do(req)
	if err != nil {
		return errors.WithStack(netdiag.New("send JSON request", url, err))
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

// checkRespStatus is the first CheckResp chaining step; it expects the chain value c to hold an *http.Response.
// It returns the response with a nil error for 2xx status codes, the response with an HTTPInvalidStatusError
// for any other status code, and a nil value with an error when the chain value is not an *http.Response.
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

// checkRespErr builds the CheckResp chaining step that enriches an upstream status error with the response
// body. maxErrBodyBytes caps how many body bytes are read into the error message. The returned step passes the
// chain value through unchanged when there is no upstream error; otherwise it closes the response body and
// returns the response with the upstream error wrapped as "got http body: ..." (marked "(truncated)" when
// the body exceeds the limit), or wrapped with the read failure if the body cannot be read. A chain value that
// is not an *http.Response yields a nil value and the upstream error joined with a type error.
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
