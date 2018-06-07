package utils_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/go-utils"
)

func TestRequestJSON(t *testing.T) {
	data := utils.RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	want := "{map[hello:world]}"
	utils.RequestJSON("POST", "http://httpbin.org/post", &data, &resp)
	if fmt.Sprintf("%v", resp) != want {
		t.Errorf("got: %v", resp)
	}
}
func TestRequestJSONWithClient(t *testing.T) {
	data := utils.RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	want := "{map[hello:world]}"
	httpClient := &http.Client{}
	utils.RequestJSONWithClient(httpClient, "POST", "http://httpbin.org/post", &data, &resp)
	if fmt.Sprintf("%v", resp) != want {
		t.Errorf("got: %v", resp)
	}
}

func TestCheckResp(t *testing.T) {
	var (
		resp *http.Response
		err  error
	)
	resp = &http.Response{
		StatusCode: 500,
		Body:       ioutil.NopCloser(bytes.NewBufferString(`some error message`)),
	}
	err = utils.CheckResp(resp)
	if err == nil {
		t.Error("missing error")
	}
	if !strings.Contains(err.Error(), "some error message") {
		t.Errorf("error message error <%v>", err.Error())
	}
}
