package compress

import (
	"archive/zip"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
)

const (
	defaultUnzipMaxEntries = 100000
	// DefaultUnzipMaxBytes caps aggregate decompressed bytes for a default extraction.
	DefaultUnzipMaxBytes int64 = 64 * 1024 * 1024
	// DefaultUnzipMaxFileBytes caps a single decompressed file by default.
	DefaultUnzipMaxFileBytes int64 = 16 * 1024 * 1024
)

// ErrUnzipByteLimit reports a decompressed byte-budget violation.
var ErrUnzipByteLimit = errors.New("decompressed bytes exceed configured limit")

// unzipOption holds finite extraction budgets; zero byte budgets require explicit opt-out.
type unzipOption struct {
	maxBytes       int64
	maxFileBytes   int64
	copyChunkBytes int64
	maxEntries     int
}

// fillDefault installs conservative aggregate, per-entry and buffer limits.
func (o *unzipOption) fillDefault() *unzipOption {
	o.maxBytes = DefaultUnzipMaxBytes
	o.maxFileBytes = DefaultUnzipMaxFileBytes
	o.copyChunkBytes = 32 * 1024
	o.maxEntries = defaultUnzipMaxEntries
	return o
}

// applyOpts validates and applies options in caller order.
func (o *unzipOption) applyOpts(fs ...UnzipOption) (*unzipOption, error) {
	for _, f := range fs {
		if f == nil {
			return nil, errors.New("unzip option must not be nil")
		}
		if err := f(o); err != nil {
			return nil, errors.Wrap(err, "apply unzip option")
		}
	}
	return o, nil
}

// UnzipOption changes an extraction policy before any files are written.
type UnzipOption func(*unzipOption) error

// UnzipWithMaxBytes sets a positive aggregate budget; it does not disable the per-file cap.
func UnzipWithMaxBytes(n int64) UnzipOption {
	return func(o *unzipOption) error {
		if n < 1 {
			return errors.New("max bytes must >= 1")
		}
		o.maxBytes = n
		return nil
	}
}

// UnzipWithMaxFileBytes sets a positive decompressed budget for each file.
func UnzipWithMaxFileBytes(n int64) UnzipOption {
	return func(o *unzipOption) error {
		if n < 1 {
			return errors.New("max file bytes must >= 1")
		}
		o.maxFileBytes = n
		return nil
	}
}

// UnzipWithUnsafeUnlimitedBytes disables both byte caps for explicitly trusted archives.
// Entry and copy-buffer limits remain active. Subsequent options can restore byte caps.
func UnzipWithUnsafeUnlimitedBytes() UnzipOption {
	return func(o *unzipOption) error { o.maxBytes = 0; o.maxFileBytes = 0; return nil }
}

// UnzipWithCopyChunkBytes selects a buffer size from one byte through one MiB.
func UnzipWithCopyChunkBytes(n int64) UnzipOption {
	return func(o *unzipOption) error {
		if n < 1 || n > 1024*1024 {
			return errors.New("copy chunk bytes must be between 1 and 1048576")
		}
		o.copyChunkBytes = n
		return nil
	}
}

// UnzipWithMaxEntries limits archive entries. Zero explicitly disables the entry cap.
func UnzipWithMaxEntries(n int) UnzipOption {
	return func(o *unzipOption) error {
		if n < 0 {
			return errors.New("max entries must >= 0")
		}
		o.maxEntries = n
		return nil
	}
}

// Unzip extracts into a nonempty trusted directory with finite aggregate and per-file
// defaults. Names and advertised sizes are checked before writes; actual decompressed
// bytes are limited independently. Each file is published only after complete reading,
// checksum verification and close. Earlier successful files remain after later failure.
// Parent-directory symlink confinement is not provided by lexical path validation.
func Unzip(src, dest string, opts ...UnzipOption) (filenames []string, retErr error) {
	if dest == "" {
		return nil, errors.New("trusted unzip destination must not be empty")
	}
	o, err := new(unzipOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply unzip options")
	}
	r, err := zip.OpenReader(src)
	if err != nil {
		return nil, errors.Wrap(err, "open ZIP archive")
	}
	defer func() {
		if err := r.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "close ZIP archive"))
		}
	}()
	if o.maxEntries > 0 && len(r.File) > o.maxEntries {
		return nil, errors.Errorf("zip entries %d exceed max entries limit %d", len(r.File), o.maxEntries)
	}
	var advertised uint64
	for _, f := range r.File {
		if err := validateZIPMemberName(f.Name); err != nil {
			return nil, errors.Wrap(err, "invalid ZIP member name")
		}
		if _, err := gutils.JoinFilepath(dest, f.Name); err != nil {
			return nil, errors.Wrap(err, "ZIP member escapes trusted destination")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if o.maxFileBytes > 0 && f.UncompressedSize64 > uint64(o.maxFileBytes) {
			return nil, errors.Wrap(ErrUnzipByteLimit, "decompressed bytes exceed per-file limit")
		}
		if o.maxBytes > 0 {
			if f.UncompressedSize64 > uint64(o.maxBytes)-advertised {
				return nil, errors.Wrap(ErrUnzipByteLimit, "decompressed bytes exceed aggregate limit")
			}
			advertised += f.UncompressedSize64
		}
	}
	var total int64
	for _, f := range r.File {
		name, err := gutils.JoinFilepath(dest, f.Name)
		if err != nil {
			return nil, errors.Wrap(err, "resolve ZIP member")
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(name, 0751); err != nil {
				return nil, errors.Wrap(err, "create ZIP directory")
			}
		} else if err := unzipFile(f, name, o, &total); err != nil {
			return nil, errors.Wrapf(err, "extract ZIP member %q", f.Name)
		}
		filenames = append(filenames, name)
	}
	return filenames, nil
}

// unzipFile verifies a member into a private temporary file before replacing its target.
// Failed members never truncate an existing target or publish incomplete output.
func unzipFile(f *zip.File, name string, o *unzipOption, total *int64) (retErr error) {
	if *total < 0 || o.maxBytes > 0 && *total > o.maxBytes {
		return errors.WithStack(ErrUnzipByteLimit)
	}
	limit := int64(-1)
	label := "aggregate"
	if o.maxBytes > 0 {
		limit = o.maxBytes - *total
	}
	if o.maxFileBytes > 0 && (limit < 0 || o.maxFileBytes < limit) {
		limit = o.maxFileBytes
		label = "per-file"
	}
	src, err := f.Open()
	if err != nil {
		return errors.Wrap(err, "open compressed member")
	}
	srcClosed := false
	defer func() {
		if !srcClosed {
			if err := src.Close(); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "close compressed member"))
			}
		}
	}()
	parent := filepath.Dir(name)
	if err := os.MkdirAll(parent, 0751); err != nil {
		return errors.Wrap(err, "create extraction directory")
	}
	out, err := os.CreateTemp(parent, ".unzip-*")
	if err != nil {
		return errors.Wrap(err, "create private extraction file")
	}
	temp := out.Name()
	closed, published := false, false
	defer func() {
		if !closed {
			if err := out.Close(); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "close incomplete extraction"))
			}
		}
		if !published {
			if err := os.Remove(temp); err != nil && !os.IsNotExist(err) {
				retErr = errors.Join(retErr, errors.Wrap(err, "remove incomplete extraction"))
			}
		}
	}()
	n, err := copyZIPLimited(out, src, limit, int(o.copyChunkBytes))
	if n > math.MaxInt64-*total {
		return errors.New("decompressed byte counter overflow")
	}
	*total += n
	if errors.Is(err, ErrUnzipByteLimit) {
		return errors.Wrapf(err, "decompressed bytes exceed %s limit", label)
	}
	if err != nil {
		return errors.Wrap(err, "read complete ZIP member")
	}
	err = src.Close()
	srcClosed = true
	if err != nil {
		return errors.Wrap(err, "close verified compressed member")
	}
	if err := out.Chmod(f.Mode().Perm() & 0755); err != nil {
		return errors.Wrap(err, "set extracted permissions")
	}
	err = out.Close()
	closed = true
	if err != nil {
		return errors.Wrap(err, "close verified extraction")
	}
	if err := os.Rename(temp, name); err != nil {
		return errors.Wrap(err, "publish verified extraction")
	}
	published = true
	return nil
}

// copyZIPLimited probes a single extra byte in memory, never beyond the output budget.
// A negative limit is the explicit unlimited policy; buffer allocation stays bounded.
func copyZIPLimited(dst io.Writer, src io.Reader, limit int64, chunk int) (int64, error) {
	if chunk < 1 || chunk > 1024*1024 {
		return 0, errors.New("invalid extraction copy buffer")
	}
	buf := make([]byte, chunk)
	if limit < 0 {
		n, err := io.CopyBuffer(struct{ io.Writer }{dst}, struct{ io.Reader }{src}, buf)
		return n, errors.Wrap(err, "copy unlimited trusted ZIP member")
	}
	n, err := io.CopyBuffer(struct{ io.Writer }{dst}, io.LimitReader(src, limit), buf)
	if err != nil {
		return n, errors.Wrap(err, "copy bounded ZIP member")
	}
	var probe [1]byte
	extra, err := io.ReadFull(src, probe[:])
	if extra != 0 {
		return n, errors.WithStack(ErrUnzipByteLimit)
	}
	if errors.Is(err, io.EOF) {
		return n, nil
	}
	return n, errors.Wrap(err, "verify end of ZIP member")
}
