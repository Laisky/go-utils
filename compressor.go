package utils

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/klauspost/pgzip"
	"github.com/pkg/errors"
)

const (
	defaultGzCompressLevel      = gzip.DefaultCompression
	defaultPGzCompressLevel     = pgzip.DefaultCompression
	defaultCompressBufSizeByte  = 4 * 1024 * 1024
	defaultPgzCompressNBlock    = 16
	defaultPgzCompressBlockSize = 250000
)

// CompressorItf interface of compressor
type CompressorItf interface {
	Write([]byte) (int, error)
	WriteString(string) (int, error)
	// write footer and flust to lower writer
	Flush() error
	// write footer without flush
	WriteFooter() error
}

type compressOption struct {
	level, bufSizeByte,
	nBlock, blockSizeByte int
}

// CompressOptFunc options for compressor
type CompressOptFunc func(*compressOption) error

// GZCompressor compress by gz with buf
type GZCompressor struct {
	*compressOption
	buf      *bufio.Writer
	gzWriter *gzip.Writer
	writer   io.Writer
}

// WithCompressBufSizeByte set compressor buf size
func WithCompressBufSizeByte(n int) CompressOptFunc {
	return func(opt *compressOption) error {
		if n < 0 {
			return fmt.Errorf("`BufSizeByte` should great than or equal to 0")
		}

		opt.bufSizeByte = n
		return nil
	}
}

// WithCompressLevel set compressor compress level
func WithCompressLevel(n int) CompressOptFunc {
	return func(opt *compressOption) error {
		opt.level = n
		return nil
	}
}

// NewGZCompressor create new GZCompressor
func NewGZCompressor(writer io.Writer, opts ...CompressOptFunc) (c *GZCompressor, err error) {
	opt := &compressOption{
		level:       defaultGzCompressLevel,
		bufSizeByte: defaultCompressBufSizeByte,
	}
	for _, of := range opts {
		if err = of(opt); err != nil {
			return nil, errors.Wrap(err, "set option")
		}
	}
	c = &GZCompressor{
		writer:         writer,
		compressOption: opt,
	}
	c.buf = bufio.NewWriterSize(c.writer, c.bufSizeByte)
	if c.gzWriter, err = gzip.NewWriterLevel(c.buf, c.level); err != nil {
		return nil, err
	}

	return c, nil
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

// PGZCompressor compress by gz with buf
type PGZCompressor struct {
	*compressOption
	buf      *bufio.Writer
	gzWriter *pgzip.Writer
	writer   io.Writer
}

// WithPGzipNBlocks set compressor blocks
func WithPGzipNBlocks(nBlock int) CompressOptFunc {
	return func(opt *compressOption) error {
		if nBlock < 0 {
			return fmt.Errorf("nBlock size must greater than 0, got %v", nBlock)
		}

		opt.nBlock = nBlock
		return nil
	}
}

// WithPGzipBlockSize set compressor blocks
func WithPGzipBlockSize(bytes int) CompressOptFunc {
	return func(opt *compressOption) error {
		if bytes <= 0 {
			return fmt.Errorf("block size must greater than 0, got %v", bytes)
		}

		opt.blockSizeByte = bytes
		return nil
	}
}

// NewPGZCompressor create new PGZCompressor
func NewPGZCompressor(writer io.Writer, opts ...CompressOptFunc) (c *PGZCompressor, err error) {
	opt := &compressOption{
		level:         defaultPGzCompressLevel,
		bufSizeByte:   defaultCompressBufSizeByte,
		nBlock:        defaultPgzCompressNBlock,
		blockSizeByte: defaultPgzCompressBlockSize,
	}
	for _, of := range opts {
		if err = of(opt); err != nil {
			return nil, errors.Wrap(err, "set option")
		}
	}
	c = &PGZCompressor{
		writer:         writer,
		compressOption: opt,
	}
	c.buf = bufio.NewWriterSize(c.writer, c.bufSizeByte)
	if c.gzWriter, err = pgzip.NewWriterLevel(c.buf, c.level); err != nil {
		return nil, errors.Wrap(err, "new pgzip")
	}
	if err = c.gzWriter.SetConcurrency(opt.blockSizeByte, opt.nBlock); err != nil {
		return nil, errors.Wrap(err, "set pgzip concurency")
	}

	return c, nil
}

// Write write bytes via compressor
func (c *PGZCompressor) Write(d []byte) (int, error) {
	return c.gzWriter.Write(d)
}

// WriteString write string via compressor
func (c *PGZCompressor) WriteString(d string) (int, error) {
	return c.gzWriter.Write([]byte(d))
}

// Flush flush buffer bytes into bottom writer with gz meta footer
func (c *PGZCompressor) Flush() (err error) {
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
func (c *PGZCompressor) WriteFooter() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return err
	}
	c.gzWriter.Reset(c.buf)
	return nil
}
