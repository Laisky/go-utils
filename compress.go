package utils

import (
	"bufio"
	"compress/gzip"
	"io"
)

// GZCompressorCfg configuration for GZCompressor
type GZCompressorCfg struct {
	BufSizeByte int       // buf size in bytes
	Writer      io.Writer // bottom writer
}

// GZCompressor compress by gz with buf
type GZCompressor struct {
	*GZCompressorCfg
	buf      *bufio.Writer
	gzWriter *gzip.Writer
}

// NewGZCompressor create new GZCompressor
func NewGZCompressor(cfg *GZCompressorCfg) *GZCompressor {
	if cfg.BufSizeByte < 0 {
		Logger.Panic("`BufSizeByte` should great than or equal to 0")
	}

	c := &GZCompressor{
		GZCompressorCfg: cfg,
	}
	c.buf = bufio.NewWriterSize(c.Writer, c.BufSizeByte)
	c.gzWriter = gzip.NewWriter(c.buf)
	return c
}

// Write write bytes via compressor
func (c *GZCompressor) Write(d []byte) (int, error) {
	return c.gzWriter.Write(d)
}

// WriteString write string via compressor
func (c *GZCompressor) WriteString(d string) (int, error) {
	return c.gzWriter.Write([]byte(d))
}

// Flush flush buffer bytes into bottom writer with gz meta footer
func (c *GZCompressor) Flush() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return err
	}
	if err = c.buf.Flush(); err != nil {
		return err
	}
	c.gzWriter.Reset(c.buf)
	return nil
}

// WriteFooter write gz footer
func (c *GZCompressor) WriteFooter() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return err
	}
	c.gzWriter.Reset(c.buf)
	return nil
}
