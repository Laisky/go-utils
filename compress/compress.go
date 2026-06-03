// Package compress contains some useful tools to compress/decompress data or files
package compress

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/klauspost/pgzip"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/log"
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

type gzDecompressOption struct {
	maxBytes int64
}

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

// defaultUnzipMaxEntries is the default cap on the number of entries that
// will be extracted from a zip archive. It guards against archives that pack
// an excessive number of entries (a zip-bomb / resource-exhaustion vector)
// while remaining far above any realistic legitimate archive size.
const defaultUnzipMaxEntries = 100000

type unzipOption struct {
	maxBytes       int64
	copyChunkBytes int64
	maxEntries     int
}

func (o *unzipOption) fillDefault() *unzipOption {
	o.copyChunkBytes = 32 * 1024
	o.maxEntries = defaultUnzipMaxEntries
	return o
}

func (o *unzipOption) applyOpts(optfs ...UnzipOption) (*unzipOption, error) {
	for _, f := range optfs {
		if err := f(o); err != nil {
			return nil, errors.Wrap(err, "apply unzip option")
		}
	}

	return o, nil
}

// UnzipOption optional arguments for UnZip
type UnzipOption func(*unzipOption) error

// UnzipWithMaxBytes the aggregate decompressed bytes across ALL entries in the
// archive will not exceed this limit, default/0 is unlimit.
//
// Unlike the previous behavior (which applied this limit per-entry and silently
// truncated oversized entries), the limit is now an aggregate cap over the whole
// archive and any entry that would exceed the remaining budget makes Unzip return
// an explicit error. This avoids the decompression-bomb / disk-exhaustion DoS
// where total extracted bytes = (#entries) x maxBytes.
func UnzipWithMaxBytes(bytes int64) UnzipOption {
	return func(o *unzipOption) error {
		if bytes < 1 {
			return errors.Errorf("max bytes must >= 1")
		}

		o.maxBytes = bytes
		return nil
	}
}

// UnzipWithCopyChunkBytes copy chunk by chunk from src to dst
func UnzipWithCopyChunkBytes(bytes int64) UnzipOption {
	return func(o *unzipOption) error {
		if bytes < 1 {
			return errors.Errorf("copy chunk bytes must >= 1")
		}

		o.copyChunkBytes = bytes
		return nil
	}
}

// UnzipWithMaxEntries limits the number of entries (files and directories)
// that will be extracted from the archive.
//
// This guards the r.File loop against archives that pack an excessive number
// of entries (a resource-exhaustion / zip-bomb vector). default is 100000,
// set 0 to unlimit.
func UnzipWithMaxEntries(n int) UnzipOption {
	return func(o *unzipOption) error {
		if n < 0 {
			return errors.Errorf("max entries must >= 0")
		}

		o.maxEntries = n
		return nil
	}
}

// Unzip will decompress a zip archive, moving all files and folders
// within the zip file (parameter 1) to an output directory (parameter 2).
//
// inspired by https://golangcode.com/unzip-files-in-go/
//
// Args:
//   - src: is the source zip file.
//   - dest: is the destination directory.
//   - (opt) UnzipWithMaxBytes: the aggregate decompressed bytes across all
//     entries will not exceed this limit, default/0 is unlimit. it's better to
//     set this value to avoid decompression bomb.
//   - (opt) UnzipWithCopyChunkBytes: copy chunk by chunk from src to dst
//   - (opt) UnzipWithMaxEntries: limit the number of entries to extract,
//     default is 100000, set 0 to unlimit.
//
// Returns:
//   - filenames: all filenames in zip file
func Unzip(src string, dest string, opts ...UnzipOption) (filenames []string, err error) {
	o, err := new(unzipOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply unzip options")
	}

	var r *zip.ReadCloser
	if r, err = zip.OpenReader(src); err != nil {
		return nil, errors.Wrap(err, "open src")
	}
	defer gutils.LogErr(r.Close, log.Shared)

	// Reject archives with an excessive number of entries before doing any
	// extraction. A crafted archive can pack a huge number of entries to
	// exhaust file descriptors / inodes / disk; cap it explicitly.
	if o.maxEntries > 0 && len(r.File) > o.maxEntries {
		return nil, errors.Errorf("zip entries %d exceed max entries limit %d",
			len(r.File), o.maxEntries)
	}

	// totalWritten accumulates the decompressed bytes across all entries so
	// that o.maxBytes is enforced as an aggregate cap over the whole archive
	// rather than per-entry. This blocks the disk-exhaustion DoS where total
	// extracted bytes = (#entries) x maxBytes.
	var totalWritten int64
	for _, f := range r.File {
		fpath, err := gutils.JoinFilepath(dest, f.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "join path: %s", f.Name)
		}

		filenames = append(filenames, fpath)
		if f.FileInfo().IsDir() {
			// Make Folder
			if err = os.MkdirAll(fpath, 0o751); err != nil {
				return nil, errors.Wrapf(err, "create basedir: %s", fpath)
			}

			log.Shared.Debug("create basedir", zap.String("path", fpath))
			continue
		}

		if err = unzipFile(f, fpath, o.maxBytes, &totalWritten); err != nil {
			return nil, errors.Wrapf(err, "extract file: %s", f.Name)
		}
	}

	return filenames, nil
}

// unzipFile extracts a single file from the zip archive.
// File descriptors are properly closed when this function returns,
// avoiding resource leaks when called in a loop.
//
// totalWritten is a running counter of the decompressed bytes already written
// across all previously extracted entries; it is updated in place so that the
// caller can enforce maxBytes as an aggregate cap over the whole archive.
func unzipFile(f *zip.File, fpath string, maxBytes int64, totalWritten *int64) error {
	if err := os.MkdirAll(filepath.Dir(fpath), 0o751); err != nil {
		return errors.Wrapf(err, "mkdir: %s", fpath)
	}
	log.Shared.Debug("create basedir", zap.String("path", filepath.Dir(fpath)))

	// Clamp the archive-controlled permission bits before creating the file.
	// A malicious archive could otherwise request setuid/setgid/sticky or
	// group/other-writable modes; restrict to owner rwx + group/other rx.
	mode := f.Mode().Perm() & 0o755
	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return errors.Wrapf(err, "open file to write: %s", fpath)
	}
	defer gutils.SilentClose(outFile)
	log.Shared.Debug("create file", zap.String("path", fpath))

	compressedFp, err := f.Open()
	if err != nil {
		return errors.Wrapf(err, "read src file to write: %s", f.Name)
	}
	defer gutils.SilentClose(compressedFp)

	if maxBytes > 0 {
		// Enforce the aggregate limit explicitly. Copy at most (remaining+1)
		// bytes so that an entry exceeding the remaining budget is detected and
		// reported as an error instead of being silently truncated (the old
		// io.Copy(LimitReader) behavior hid oversized/decompression-bomb data).
		remaining := maxBytes - *totalWritten
		if remaining < 0 {
			remaining = 0
		}

		var n int64
		n, err = io.CopyN(outFile, compressedFp, remaining+1)
		*totalWritten += n
		if err == nil {
			// We were able to read remaining+1 bytes, so the aggregate output
			// exceeds maxBytes.
			return errors.Errorf("decompressed bytes exceed aggregate limit %d", maxBytes)
		}
		if !errors.Is(err, io.EOF) {
			return errors.Wrapf(err, "copy file: %s", f.Name)
		}
		// io.EOF means the entry finished within budget: normal completion.
	} else {
		var n int64
		// user did not set maxBytes, so it's user's responsibility to avoid
		// decompression bomb
		n, err = io.Copy(outFile, compressedFp) //nolint:gosec // see comment above
		*totalWritten += n
		if err != nil {
			return errors.Wrapf(err, "copy file: %s", f.Name)
		}
	}

	return nil
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
		return errors.Wrapf(err, "create zip file %q", output)
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
		fs, err := os.ReadDir(filename)
		if err != nil {
			return errors.Wrapf(err, "list files in `%s`", filename)
		}

		for _, finfoInDir := range fs {
			_, childDir := filepath.Split(finfoInDir.Name())

			nextFilename, err := gutils.JoinFilepath(filename, finfoInDir.Name())
			if err != nil {
				return errors.Wrapf(err, "join nextFilename filepath `%s`", finfoInDir.Name())
			}
			nextBasedir, err := gutils.JoinFilepath(basedir, finfo.Name())
			if err != nil {
				return errors.Wrapf(err, "join nextBasedir filepath `%s`", childDir)
			}

			if err = AddFileToZip(zipWriter, nextFilename, nextBasedir); err != nil {
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
		if header.Name, err = gutils.JoinFilepath(basedir, finfo.Name()); err != nil {
			return errors.Wrapf(err, "join filepath `%s`", finfo.Name())
		}
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
