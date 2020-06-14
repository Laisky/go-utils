package utils

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Laisky/zap"
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
			return fmt.Errorf("nBlock size must greater than 0, got %d", nBlock)
		}

		opt.nBlock = nBlock
		return nil
	}
}

// WithPGzipBlockSize set compressor blocks
func WithPGzipBlockSize(bytes int) CompressOptFunc {
	return func(opt *compressOption) error {
		if bytes <= 0 {
			return fmt.Errorf("block size must greater than 0, got %d", bytes)
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

// Unzip will decompress a zip archive, moving all files and folders
// within the zip file (parameter 1) to an output directory (parameter 2).
//
// https://golangcode.com/unzip-files-in-go/
func Unzip(src string, dest string) (filenames []string, err error) {
	var r *zip.ReadCloser
	if r, err = zip.OpenReader(src); err != nil {
		return nil, errors.Wrap(err, "open src")
	}
	defer r.Close()

	for _, f := range r.File {
		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: https://snyk.io/research/zip-slip-vulnerability#go
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("illegal file path: %s", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			if err = os.MkdirAll(fpath, os.ModePerm); err != nil {
				return nil, errors.Wrapf(err, "create dir: %s", fpath)
			}

			Logger.Debug("create dir", zap.String("path", fpath))
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return nil, errors.Wrapf(err, "mkdir: %s", fpath)
		}
		Logger.Debug("create dir", zap.String("path", filepath.Dir(fpath)))

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return nil, errors.Wrapf(err, "open file to write: %s", fpath)
		}
		Logger.Debug("create file", zap.String("path", filepath.Dir(fpath)))
		defer outFile.Close()

		rc, err := f.Open()
		if err != nil {
			return nil, errors.Wrapf(err, "read src file to write: %s", f.Name)
		}
		defer rc.Close()

		if _, err = io.Copy(outFile, rc); err != nil {
			return nil, errors.Wrap(err, "copy src to dest")
		}
	}

	return filenames, nil
}

// ZipFiles compresses one or many files into a single zip archive file.
// Param 1: filename is the output zip file's name.
// Param 2: files is a list of files to add to the zip.
//
// https://golangcode.com/create-zip-files-in-go/
func ZipFiles(filename string, files []string) (err error) {
	var newZipFile *os.File
	if newZipFile, err = os.Create(filename); err != nil {
		return err
	}
	defer newZipFile.Close()

	zipWriter := zip.NewWriter(newZipFile)
	defer zipWriter.Close()

	// Add files to zip
	for _, file := range files {
		if err = AddFileToZip(zipWriter, file); err != nil {
			return errors.Wrapf(err, "AddFileToZip: %s", file)
		}
		Logger.Debug("add file to zip", zap.String("file", file))
	}

	return nil
}

// AddFileToZip add file tp zip.Writer
//
// https://golangcode.com/create-zip-files-in-go/
func AddFileToZip(zipWriter *zip.Writer, filename string) (err error) {
	var fileToZip *os.File
	if fileToZip, err = os.Open(filename); err != nil {
		return errors.Wrapf(err, "open file: %s", filename)
	}
	defer fileToZip.Close()

	// Get the file information
	var info os.FileInfo
	if info, err = fileToZip.Stat(); err != nil {
		return errors.Wrapf(err, "get file stat: %s", filename)
	}

	var header *zip.FileHeader
	if header, err = zip.FileInfoHeader(info); err != nil {
		return errors.Wrap(err, "get file header")
	}

	// Using FileInfoHeader() above only uses the basename of the file. If we want
	// to preserve the folder structure we can overwrite this with the full path.
	// header.Name = filename

	// Change to deflate to gain better compression
	// see http://golang.org/pkg/archive/zip/#pkg-constants
	header.Method = zip.Deflate

	var writer io.Writer
	if writer, err = zipWriter.CreateHeader(header); err != nil {
		return errors.Wrap(err, "create writer header")
	}

	if _, err = io.Copy(writer, fileToZip); err != nil {
		return errors.Wrap(err, "copy data")
	}

	return
}
