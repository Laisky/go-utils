package utils

import (
	"bytes"
	"context"
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
	t.Parallel()

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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewJaegerTracingID(tt.traceID, tt.spanID, tt.parentSpanID, tt.flag)
			require.NoError(t, err)
			if tt.wantErr {
				require.NotEqual(t, tt.want.String(), got.String())
				_, _, _, _, err = got.Parse()
				require.NoError(t, err)
				return
			}

			require.Equal(t, tt.want.String(), got.String())

			traceID, spanID, parentSpanID, flag, err := got.Parse()
			require.NoError(t, err)
			require.Equal(t, tt.traceID, traceID)
			require.Equal(t, tt.spanID, spanID)
			require.Equal(t, tt.parentSpanID, parentSpanID)
			require.Equal(t, tt.flag, flag)
		})
	}
}

func TestOpenURLInDefaultBrowser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := OpenURLInDefaultBrowser(ctx, "https://www.example.com")
	require.NoError(t, err)
}
