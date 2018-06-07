package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/go-chaining"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

var (
	httpCliend = &http.Client{ // default http client
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 20,
		},
		Timeout: time.Duration(30) * time.Second,
	}
)

// RequestData 发起请求的结构体
type RequestData struct {
	Headers map[string]string
	Data    interface{}
}

// RequestJSON request JSON and return JSON by default client
func RequestJSON(method, url string, request *RequestData, resp interface{}) (err error) {
	return RequestJSONWithClient(httpCliend, method, url, request, resp)
}

// RequestJSONWithClient request JSON and return JSON with specific client
func RequestJSONWithClient(httpClient *http.Client, method, url string, request *RequestData, resp interface{}) (err error) {
	log.Debugf("RequestJSON for method %v, url %v, data %+v", method, url, request)

	var (
		jsonBytes []byte
	)
	jsonBytes, err = json.Marshal(request.Data)
	log.Debugf("request json %v", string(jsonBytes[:]))
	if err != nil {
		return errors.Wrap(err, "marshal request data error")
	}

	req, err := http.NewRequest(strings.ToUpper(method), url, bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range request.Headers {
		req.Header.Set(k, v)
	}

	r, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "try to request url error")
	} else if (r.StatusCode < 200) && (r.StatusCode >= 300) {
		defer r.Body.Close()
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
	err = json.Unmarshal(respBytes, resp)
	if err != nil {
		errMsg := fmt.Sprintf("try to unmarshal response data error: %v\n%v", err, string(respBytes[:]))
		return errors.Wrap(err, errMsg)
	}
	log.Debugf("RequestJSON return: %+v", string(respBytes[:]))

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
	if FloorDivision(code, 100) != 2 {
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

	return resp, errors.Wrapf(upErr, "got http body <%v>", string(respB[:]))
}
