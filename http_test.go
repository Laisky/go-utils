package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestRequestJSON(t *testing.T) {
	data := RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	want := "{map[hello:world]}"
	if err := RequestJSON("POST", "http://httpbin.org/post", &data, &resp); err != nil {
		t.Fatalf("got: %v", resp)
	}
	if fmt.Sprintf("%v", resp) != want {
		t.Fatalf("got: %v", resp)
	}
}
func TestRequestJSONWithClient(t *testing.T) {
	data := RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	want := "{map[hello:world]}"
	httpClient := &http.Client{}
	if err := RequestJSONWithClient(httpClient, "POST", "http://httpbin.org/post", &data, &resp); err != nil {
		t.Fatalf("got: %v", resp)
	}
	if fmt.Sprintf("%v", resp) != want {
		t.Fatalf("got: %v", resp)
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
	err = CheckResp(resp)
	if err == nil {
		t.Error("missing error")
	}
	if !strings.Contains(err.Error(), "some error message") {
		t.Errorf("error message error <%v>", err.Error())
	}
}
