// Package compress contains some useful tools to compress/decompress data or files
package compress

import (
	"bufio"
	"compress/gzip"
	"io"

	"github.com/Laisky/errors/v2"
	"github.com/klauspost/pgzip"
)

const (
	defaultGzipLevel      = gzip.DefaultCompression
	defaultPGzipLevel     = pgzip.DefaultCompression
	defaultBufSizeByte    = 4 * 1024 * 1024
	defaultPgzipNBlock    = 16
	defaultPgzipBlockSize = 250000
)

// GzCompress compress data by gzip
func GzCompress(in io.Reader, out io.Writer) error {
	gz, err := NewGZip(out)
	if err != nil {
		return errors.Wrap(err, "new gzip")
	}

	_, err = io.Copy(gz, in)
	if err != nil {
		return errors.Wrap(err, "copy data")
	}

	if err = gz.WriteFooter(); err != nil {
		return errors.Wrap(err, "write footer")
	}

	return gz.Flush()
}

// gzDecompressOption holds the settings used by GzDecompress, currently the maximum number of decompressed bytes it
// may write.
type gzDecompressOption struct {
	maxBytes int64
}

// apply resets o to its defaults (a 1GB decompression limit) and then applies each option in fs in order. It returns o
// configured with the options, or an error wrapping the first option that fails validation.
func (o *gzDecompressOption) apply(fs ...GzDecompressOption) (*gzDecompressOption, error) {
	// set default
	o.maxBytes = 1 * 1024 * 1024 * 1024 // 1GB

	// apply opts
	for _, f := range fs {
		if err := f(o); err != nil {
			return nil, errors.Wrap(err, "apply gz decompress option")
		}
	}

	return o, nil
}

// GzDecompressOption optional arguments for GzDecompress
type GzDecompressOption func(*gzDecompressOption) error

// WithGzDecompressMaxBytes decompressed bytes will not exceed this limit,
//
// default is 1GB, it's better to set this value to avoid decompression bomb.
// set 0 to unlimit.
func WithGzDecompressMaxBytes(bytes int64) GzDecompressOption {
	return func(o *gzDecompressOption) error {
		if bytes <= 0 {
			return errors.Errorf("max bytes must >= 0")
		}

		o.maxBytes = bytes
		return nil
	}
}

// GzDecompress decompress data by gzip
//
// be careful about the decompression bomb, see https://en.wikipedia.org/wiki/Zip_bomb,
// default maxBytes is 1GB, it's better to set this value to avoid decompression bomb.
func GzDecompress(in io.Reader, out io.Writer, opts ...GzDecompressOption) error {
	opt, err := new(gzDecompressOption).apply(opts...)
	if err != nil {
		return errors.Wrap(err, "apply opts")
	}

	gz, err := gzip.NewReader(in)
	if err != nil {
		return errors.Wrap(err, "new gzip reader")
	}

	chunk := make([]byte, 4*1024*1024)
	var totalBytes int64
	for {
		n, err := gz.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return errors.Wrap(err, "read gzip")
		}

		if totalBytes += int64(n); totalBytes > opt.maxBytes {
			return errors.Errorf("decompressed bytes %d exceed limit %d", totalBytes, opt.maxBytes)
		}

		if _, err = out.Write(chunk[:n]); err != nil {
			return errors.Wrap(err, "write decompressed data")
		}
	}

	return nil
}

// Compressor interface of compressor
type Compressor interface {
	Write([]byte) (int, error)
	WriteString(string) (int, error)
	// write footer and flust to lower writer
	Flush() error
	// write footer without flush
	WriteFooter() error
}

type option struct {
	level, bufSizeByte,
	nBlock, blockSizeByte int
}

// CompressOptFunc options for compressor
type Option func(*option) error

// GZCompressor compress by gz with buf
type Gzip struct {
	*option
	buf      *bufio.Writer
	gzWriter *gzip.Writer
	writer   io.Writer
}

// WithBufSizeByte set compressor buf size
func WithBufSizeByte(n int) Option {
	return func(opt *option) error {
		if n < 0 {
			return errors.Errorf("`BufSizeByte` should great than or equal to 0")
		}

		opt.bufSizeByte = n
		return nil
	}
}

// WithLevel set compressor compress level
func WithLevel(n int) Option {
	return func(opt *option) error {
		opt.level = n
		return nil
	}
}

// NewGZip create new GZCompressor
func NewGZip(writer io.Writer, opts ...Option) (*Gzip, error) {
	opt := &option{
		level:       defaultGzipLevel,
		bufSizeByte: defaultBufSizeByte,
	}
	var err error
	for _, of := range opts {
		if err = of(opt); err != nil {
			return nil, errors.Wrap(err, "set option")
		}
	}
	c := &Gzip{
		writer: writer,
		option: opt,
	}
	c.buf = bufio.NewWriterSize(c.writer, c.bufSizeByte)
	if c.gzWriter, err = gzip.NewWriterLevel(c.buf, c.level); err != nil {
		return nil, errors.Wrap(err, "create gzip writer")
	}

	return c, nil
}

// Write write bytes via compressor
func (c *Gzip) Write(d []byte) (int, error) {
	return c.gzWriter.Write(d)
}

// WriteString write string via compressor
func (c *Gzip) WriteString(d string) (int, error) {
	return c.gzWriter.Write([]byte(d))
}

// Flush flush buffer bytes into bottom writer with gz meta footer
func (c *Gzip) Flush() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return errors.Wrap(err, "close gzip writer")
	}
	if err = c.buf.Flush(); err != nil {
		return errors.Wrap(err, "flush buffer")
	}
	c.gzWriter.Reset(c.buf)
	return nil
}

// WriteFooter write gz footer
func (c *Gzip) WriteFooter() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return errors.Wrap(err, "close gzip writer for footer")
	}
	c.gzWriter.Reset(c.buf)
	return nil
}

// PGZip parallel gzip compressor
//
// call `NewPGZip` to create new PGZip
type PGZip struct {
	*option
	buf      *bufio.Writer
	gzWriter *pgzip.Writer
	writer   io.Writer
}

// WithPGzipNBlocks set compressor blocks
func WithPGzipNBlocks(nBlock int) Option {
	return func(opt *option) error {
		if nBlock < 0 {
			return errors.Errorf("nBlock size must greater than 0, got %d", nBlock)
		}

		opt.nBlock = nBlock
		return nil
	}
}

// WithPGzipBlockSize set compressor blocks
func WithPGzipBlockSize(bytes int) Option {
	return func(opt *option) error {
		if bytes <= 0 {
			return errors.Errorf("block size must greater than 0, got %d", bytes)
		}

		opt.blockSizeByte = bytes
		return nil
	}
}

// NewPGZip create new PGZCompressor
func NewPGZip(writer io.Writer, opts ...Option) (*PGZip, error) {
	opt := &option{
		level:         defaultPGzipLevel,
		bufSizeByte:   defaultBufSizeByte,
		nBlock:        defaultPgzipNBlock,
		blockSizeByte: defaultPgzipBlockSize,
	}
	var err error
	for _, of := range opts {
		if err = of(opt); err != nil {
			return nil, errors.Wrap(err, "set option")
		}
	}
	c := &PGZip{
		writer: writer,
		option: opt,
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
func (c *PGZip) Write(d []byte) (int, error) {
	return c.gzWriter.Write(d)
}

// WriteString write string via compressor
func (c *PGZip) WriteString(d string) (int, error) {
	return c.gzWriter.Write([]byte(d))
}

// Flush flush buffer bytes into bottom writer with gz meta footer
func (c *PGZip) Flush() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return errors.Wrap(err, "close pgzip writer")
	}
	if err = c.buf.Flush(); err != nil {
		return errors.Wrap(err, "flush pgzip buffer")
	}
	c.gzWriter.Reset(c.buf)
	return nil
}

// WriteFooter write gz footer
func (c *PGZip) WriteFooter() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return errors.Wrap(err, "close pgzip writer for footer")
	}
	c.gzWriter.Reset(c.buf)
	return nil
}
