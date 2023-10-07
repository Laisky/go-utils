package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	httpClient, err := NewHTTPClient(
		WithHTTPClientInsecure(),
		WithHTTPClientMaxConn(20),
		WithHTTPClientTimeout(30*time.Second),
	)
	require.NoError(t, err)

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
		Body:       io.NopCloser(bytes.NewBufferString(`some error message`)),
	}
	err = CheckResp(resp)
	if err == nil {
		t.Error("missing error")
	}
	if !strings.Contains(err.Error(), "some error message") {
		t.Errorf("error message error <%v>", err.Error())
	}
}
func TestJaegerTracingID(t *testing.T) {
	tests := []struct {
		name         string
		traceID      uint64
		spanID       uint64
		parentSpanID uint64
		flag         byte
		want         JaegerTracingID
		wantErr      bool
	}{
		{
			name:         "valid tracing ID",
			traceID:      123456789,
			spanID:       987654321,
			parentSpanID: 0,
			flag:         0x04,
			want:         "75bcd15:3ade68b1::4",
			wantErr:      false,
		},
		{
			name:         "invalid trace ID",
			traceID:      0,
			spanID:       987654321,
			parentSpanID: 0,
			flag:         0x04,
			want:         "",
			wantErr:      true,
		},
		{
			name:         "invalid span ID",
			traceID:      123456789,
			spanID:       0,
			parentSpanID: 0,
			flag:         0x04,
			want:         "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewJaegerTracingID(tt.traceID, tt.spanID, tt.parentSpanID, tt.flag)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("JaegerTracingID() error = %v, wantErr %v", err, tt.wantErr)
				}

				return
			}

			if got != tt.want {
				t.Errorf("JaegerTracingID() = %v, want %v", got, tt.want)
				return
			}

			traceID, spanID, parentSpanID, flag, err := got.Parse()
			require.NoError(t, err)
			require.Equal(t, tt.traceID, traceID)
			require.Equal(t, tt.spanID, spanID)
			require.Equal(t, tt.parentSpanID, parentSpanID)
			require.Equal(t, tt.flag, flag)
		})
	}
}
