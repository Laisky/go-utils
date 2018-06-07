package utils_test

import (
	"testing"
	"time"

	utils "github.com/Laisky/go-utils"
)

func TestParseTs2String(t *testing.T) {
	var (
		got    string
		layout = time.RFC3339
	)

	cases := map[int64]string{
		1:         "1970-01-01T00:00:01Z",
		100000:    "1970-01-02T03:46:40Z",
		100000000: "1973-03-03T09:46:40Z",
	}
	for ts, v := range cases {
		if got = utils.ParseTs2String(ts, layout); got != v {
			t.Errorf("expect %v, got %v", v, got)
		}
	}
}
