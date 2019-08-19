package utils_test

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"testing"

	"github.com/Laisky/go-utils"
)

func TestGZCompressor(t *testing.T) {
	originText := "fj2f32f9jp9wsif0weif20if320fi23if"
	writer := &bytes.Buffer{}
	c := utils.NewGZCompressor(&utils.GZCompressorCfg{
		BufSizeByte: 1024 * 32,
		Writer:      writer,
	})
	var err error
	if _, err = c.WriteString(originText); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if err = c.Flush(); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var gz *gzip.Reader
	if gz, err = gzip.NewReader(writer); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	if bs, err := ioutil.ReadAll(gz); err != nil {
		t.Fatalf("got error: %+v", err)
	} else {
		got := string(bs)
		if got != originText {
			t.Fatalf("got: %v", got)
		}
	}
}
