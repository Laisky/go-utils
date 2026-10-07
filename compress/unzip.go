package compress

import (
	"archive/zip"
	"crypto/rand"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"

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
// Extraction uses directory handles, not re-resolved absolute destination paths.
// The caller must trust the destination root and control mount points and directory moves.
func Unzip(src, dest string, opts ...UnzipOption) (filenames []string, retErr error) {
	if runtime.GOOS == "js" {
		return nil, errors.New("race-resistant ZIP extraction is unsupported on js")
	}
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
	if err := preflightZIPMembers(r.File, dest, o); err != nil {
		return nil, err
	}
	if len(r.File) == 0 {
		return nil, nil
	}
	// Only the caller-controlled root is resolved outside os.Root. No archive
	// member participates in this mkdir/open, and preflight has already passed.
	dest = filepath.Clean(dest)
	if err := os.MkdirAll(dest, 0751); err != nil {
		return nil, errors.Wrap(err, "create trusted extraction root")
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return nil, errors.Wrap(err, "open trusted extraction root")
	}
	defer func() {
		if err := root.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "close extraction root"))
		}
	}()
	return extractZIPMembers(root, r.File, dest, o)
}

// preflightZIPMembers validates the whole archive before any filesystem write.
// It takes the archive members, the trusted destination and the options, checks
// the entry count, every member name, destination containment, and advertised
// per-file and aggregate sizes, and returns the first violation or nil.
func preflightZIPMembers(files []*zip.File, dest string, o *unzipOption) error {
	if o.maxEntries > 0 && len(files) > o.maxEntries {
		return errors.Errorf("zip entries %d exceed max entries limit %d", len(files), o.maxEntries)
	}
	var advertised uint64
	for _, f := range files {
		if err := validateZIPMemberName(f.Name); err != nil {
			return errors.Wrap(err, "invalid ZIP member name")
		}
		if _, err := gutils.JoinFilepath(dest, f.Name); err != nil {
			return errors.Wrap(err, "ZIP member escapes trusted destination")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if o.maxFileBytes > 0 && f.UncompressedSize64 > uint64(o.maxFileBytes) {
			return errors.Wrap(ErrUnzipByteLimit, "decompressed bytes exceed per-file limit")
		}
		if o.maxBytes > 0 {
			if f.UncompressedSize64 > uint64(o.maxBytes)-advertised {
				return errors.Wrap(ErrUnzipByteLimit, "decompressed bytes exceed aggregate limit")
			}
			advertised += f.UncompressedSize64
		}
	}
	return nil
}

// extractZIPMembers creates directories and extracts files through root in
// archive order. It takes the opened trusted root, the preflighted members, the
// cleaned destination (used only to report output paths) and the options, and
// returns the extracted paths or nil and the first failure. Members extracted
// before a failure remain on disk.
func extractZIPMembers(root *os.Root, files []*zip.File, dest string, o *unzipOption) ([]string, error) {
	filenames := make([]string, 0, len(files))
	var total int64
	for _, f := range files {
		name, err := gutils.JoinFilepath(dest, f.Name)
		if err != nil {
			return nil, errors.Wrap(err, "resolve ZIP member")
		}
		// Clean removes a directory entry's trailing slash before any rooted
		// operation, including OpenRoot; keep this invariant on older Go builds.
		relative := filepath.Clean(filepath.FromSlash(f.Name))
		if !filepath.IsLocal(relative) {
			return nil, errors.New("ZIP member is not local to the extraction root")
		}
		if f.FileInfo().IsDir() {
			if err := root.MkdirAll(relative, 0751); err != nil {
				return nil, errors.Wrap(err, "create ZIP directory")
			}
		} else if err := unzipFile(root, f, relative, o, &total); err != nil {
			return nil, errors.Wrapf(err, "extract ZIP member %q", f.Name)
		}
		filenames = append(filenames, name)
	}
	return filenames, nil
}

// unzipFile verifies a member into a private temporary file before replacing its target.
// Failed members never truncate an existing target or publish incomplete output.
func unzipFile(root *os.Root, f *zip.File, name string, o *unzipOption, total *int64) (retErr error) {
	limit, label, err := unzipMemberLimit(o, *total)
	if err != nil {
		return err
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
	if err := root.MkdirAll(parent, 0751); err != nil {
		return errors.Wrap(err, "create extraction directory")
	}
	// Pin the validated parent before creating the temporary file. Replacement
	// of a pathname with a symlink cannot redirect publication or cleanup.
	directory, err := root.OpenRoot(parent)
	if err != nil {
		return errors.Wrap(err, "open extraction directory")
	}
	defer func() {
		if err := directory.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "close extraction directory"))
		}
	}()
	temp := ".unzip-" + rand.Text()
	out, err := directory.OpenFile(temp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.Wrap(err, "create private extraction file")
	}
	closed, published := false, false
	defer func() {
		if !closed {
			if err := out.Close(); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "close incomplete extraction"))
			}
		}
		if !published {
			if err := directory.Remove(temp); err != nil && !os.IsNotExist(err) {
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
	if err := directory.Rename(temp, filepath.Base(name)); err != nil {
		return errors.Wrap(err, "publish verified extraction")
	}
	published = true
	return nil
}

// unzipMemberLimit computes the decompressed byte budget for the next member.
// It takes the options and the bytes already extracted, and returns the
// remaining limit (negative means the explicit unlimited policy), the label of
// the binding budget ("aggregate" or "per-file") for diagnostics, or
// ErrUnzipByteLimit when the running total is already invalid or exhausted.
func unzipMemberLimit(o *unzipOption, total int64) (limit int64, label string, err error) {
	if total < 0 || o.maxBytes > 0 && total > o.maxBytes {
		return 0, "", errors.WithStack(ErrUnzipByteLimit)
	}
	limit, label = -1, "aggregate"
	if o.maxBytes > 0 {
		limit = o.maxBytes - total
	}
	if o.maxFileBytes > 0 && (limit < 0 || o.maxFileBytes < limit) {
		limit, label = o.maxFileBytes, "per-file"
	}
	return limit, label, nil
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
