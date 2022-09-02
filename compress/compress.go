// Package compress contains some useful tools to compress/decompress data or files
package compress

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/go-utils/v2/log"
	"github.com/Laisky/zap"
	"github.com/klauspost/pgzip"
	"github.com/pkg/errors"
)

const (
	defaultGzipLevel      = gzip.DefaultCompression
	defaultPGzipLevel     = pgzip.DefaultCompression
	defaultBufSizeByte    = 4 * 1024 * 1024
	defaultPgzipNBlock    = 16
	defaultPgzipBlockSize = 250000
)

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
func NewGZip(writer io.Writer, opts ...Option) (Compressor, error) {
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
		return nil, err
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
		return err
	}
	if err = c.buf.Flush(); err != nil {
		return err
	}
	c.gzWriter.Reset(c.buf)
	return nil
}

// WriteFooter write gz footer
func (c *Gzip) WriteFooter() (err error) {
	if err = c.gzWriter.Close(); err != nil {
		return err
	}
	c.gzWriter.Reset(c.buf)
	return nil
}

// PGZip parallel gzip compressor
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
func NewPGZip(writer io.Writer, opts ...Option) (Compressor, error) {
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
		return err
	}
	if err = c.buf.Flush(); err != nil {
		return err
	}
	c.gzWriter.Reset(c.buf)
	return nil
}

// WriteFooter write gz footer
func (c *PGZip) WriteFooter() (err error) {
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
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: https://snyk.io/research/zip-slip-vulnerability#go
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, errors.Errorf("illegal file path: %s", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			if err = os.MkdirAll(fpath, os.ModePerm); err != nil {
				return nil, errors.Wrapf(err, "create basedir: %s", fpath)
			}

			log.Shared.Debug("create basedir", zap.String("path", fpath))
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return nil, errors.Wrapf(err, "mkdir: %s", fpath)
		}
		log.Shared.Debug("create basedir", zap.String("path", filepath.Dir(fpath)))

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return nil, errors.Wrapf(err, "open file to write: %s", fpath)
		}
		log.Shared.Debug("create file", zap.String("path", filepath.Dir(fpath)))
		defer gutils.SilentClose(outFile)

		rc, err := f.Open()
		if err != nil {
			return nil, errors.Wrapf(err, "read src file to write: %s", f.Name)
		}
		defer gutils.SilentClose(rc)

		if _, err = io.Copy(outFile, rc); err != nil {
			return nil, errors.Wrap(err, "copy src to dest")
		}
	}

	return filenames, nil
}

// ZipFiles compresses one or many files into a single zip archive file.
//
// Args:
//   - output: is the output zip file's name.
//   - files: is a list of files to add to the zip.
//     files can be directory.
//
// https://golangcode.com/create-zip-files-in-go/
func ZipFiles(output string, files []string) (err error) {
	var newZipFile *os.File
	if newZipFile, err = os.Create(output); err != nil {
		return err
	}
	defer gutils.SilentClose(newZipFile)

	zipWriter := zip.NewWriter(newZipFile)
	defer gutils.SilentClose(zipWriter)

	// Add files to zip
	for _, file := range files {
		if err = AddFileToZip(zipWriter, file, ""); err != nil {
			return errors.Wrapf(err, "AddFileToZip: %s", file)
		}
	}

	return nil
}

// AddFileToZip add file tp zip.Writer
//
// https://golangcode.com/create-zip-files-in-go/
func AddFileToZip(zipWriter *zip.Writer, filename, basedir string) error {
	finfo, err := os.Stat(filename)
	if err != nil {
		return errors.Wrapf(err, "get file stat: %s", filename)
	}

	if finfo.IsDir() {
		fs, err := ioutil.ReadDir(filename)
		if err != nil {
			return errors.Wrapf(err, "list files in `%s`", filename)
		}

		for _, finfoInDir := range fs {
			_, childDir := filepath.Split(finfoInDir.Name())
			if err = AddFileToZip(zipWriter,
				filepath.Join(filename, finfoInDir.Name()),
				filepath.Join(basedir, finfo.Name()),
			); err != nil {
				return errors.Wrapf(err, "zip sub basedir `%s`", childDir)
			}
		}

		return nil
	}

	fileToZip, err := os.Open(filename)
	if err != nil {
		return errors.Wrapf(err, "open file: %s", filename)
	}
	defer gutils.SilentClose(fileToZip)

	var header *zip.FileHeader
	if header, err = zip.FileInfoHeader(finfo); err != nil {
		return errors.Wrap(err, "get file header")
	}

	// Using FileInfoHeader() above only uses the basename of the file. If we want
	// to preserve the folder structure we can overwrite this with the full path.
	if basedir != "" {
		header.Name = filepath.Join(basedir, finfo.Name())
	}

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

	log.Shared.Debug("add file to zip", zap.String("file", filename))
	return nil
}
