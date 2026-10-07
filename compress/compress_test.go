package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/Laisky/go-utils/v6/log"
)

const (
	testCompressraw = "fj2f32f9jp9wsif0weif20if320fi23if"
)

func TestGZCompressor(t *testing.T) {
	t.Parallel()
	originText := testCompressraw
	writer := &bytes.Buffer{}
	c, err := NewGZip(writer)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
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

	if bs, err := io.ReadAll(gz); err != nil {
		t.Fatalf("got error: %+v", err)
	} else {
		got := string(bs)
		if got != originText {
			t.Fatalf("got: %v", got)
		}
	}
}

func ExampleNewGZip() {
	originText := testCompressraw
	writer := &bytes.Buffer{}

	var err error
	// writer
	c, err := NewGZip(
		writer,
		WithLevel(defaultGzipLevel),         // default
		WithBufSizeByte(defaultBufSizeByte), // default
	)
	if err != nil {
		log.Shared.Error("new compressor", zap.Error(err))
		return
	}
	if _, err = c.WriteString(originText); err != nil {
		log.Shared.Error("write string to compressor", zap.Error(err))
		return
	}
	if err = c.Flush(); err != nil {
		log.Shared.Error("flush compressor", zap.Error(err))
		return
	}

	// reader
	var gz *gzip.Reader
	if gz, err = gzip.NewReader(writer); err != nil {
		log.Shared.Error("new compressor", zap.Error(err))
		return
	}

	var bs []byte
	if bs, err = io.ReadAll(gz); err != nil {
		log.Shared.Error("read from compressor", zap.Error(err))
		return
	}

	got := string(bs)
	if got != originText {
		log.Shared.Error("extract compressed text invalidate",
			zap.String("got", got),
			zap.ByteString("expect", bs))
		return
	}
}

func TestPGZCompressor(t *testing.T) {
	t.Parallel()
	originText := testCompressraw
	writer := &bytes.Buffer{}
	c, err := NewPGZip(
		writer,
		WithLevel(defaultPGzipLevel),        // default
		WithBufSizeByte(defaultBufSizeByte), // default

		WithPGzipBlockSize(defaultPgzipBlockSize), // default
		WithPGzipNBlocks(defaultPgzipNBlock),      // default
	)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
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

	if bs, err := io.ReadAll(gz); err != nil {
		t.Fatalf("got error: %+v", err)
	} else {
		got := string(bs)
		if got != originText {
			t.Fatalf("got: %v", got)
		}
	}
}

func ExamplePGZip() {
	originText := testCompressraw
	writer := &bytes.Buffer{}

	var err error
	// writer
	c, err := NewPGZip(writer)
	if err != nil {
		log.Shared.Error("new compressor", zap.Error(err))
		return
	}
	if _, err = c.WriteString(originText); err != nil {
		log.Shared.Error("write string to compressor", zap.Error(err))
		return
	}
	if err = c.Flush(); err != nil {
		log.Shared.Error("flush compressor", zap.Error(err))
		return
	}

	// reader
	var gz *gzip.Reader
	if gz, err = gzip.NewReader(writer); err != nil {
		log.Shared.Error("new compressor", zap.Error(err))
		return
	}

	var bs []byte
	if bs, err = io.ReadAll(gz); err != nil {
		log.Shared.Error("read from compressor", zap.Error(err))
		return
	}

	got := string(bs)
	if got != originText {
		log.Shared.Error("extract compressed text invalidate",
			zap.String("got", got),
			zap.ByteString("expect", bs))
		return
	}
}

func TestGzCompress(t *testing.T) {
	t.Parallel()

	raw, err := gcrypto.Salt(1024 * 1024 * 10)
	require.NoError(t, err)

	var output bytes.Buffer
	err = GzCompress(bytes.NewReader(raw), &output)
	require.NoError(t, err)

	// Verify the compressed data
	var decompressOut bytes.Buffer
	err = GzDecompress(&output, &decompressOut)
	require.NoError(t, err)

	require.Equal(t, raw, decompressOut.Bytes())

	t.Run("exceed limit", func(t *testing.T) {
		var output bytes.Buffer
		err = GzCompress(bytes.NewReader(raw), &output)
		require.NoError(t, err)

		// Verify the compressed data
		var decompressOut bytes.Buffer
		err = GzDecompress(&output, &decompressOut,
			WithGzDecompressMaxBytes(1),
		)
		require.ErrorContains(t, err, "exceed limit")
	})
}
