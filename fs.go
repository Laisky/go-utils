package utils

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/fsnotify/fsnotify"

	"github.com/Laisky/go-utils/v6/internal/fileguard"
	"github.com/Laisky/go-utils/v6/log"
)

// JoinFilepath joins local child paths beneath the explicitly supplied first base.
// An empty trusted base is rejected; use "." explicitly for the current directory.
// A single nonempty base is returned unchanged. Parent components are allowed
// only when their combined, cleaned path remains within the base. Absolute or
// volume-qualified children are rejected rather than reinterpreted as relative.
// This is lexical containment only, not protection against symlinks or concurrent
// changes to filesystem objects or the process working directory.
func JoinFilepath(paths ...string) (result string, err error) {
	if len(paths) == 0 {
		return "", errors.New("empty paths")
	}
	if paths[0] == "" {
		return "", errors.New("trusted base path must not be empty")
	}
	if len(paths) == 1 {
		return paths[0], nil
	}
	for _, child := range paths[1:] {
		if filepath.IsAbs(child) || filepath.VolumeName(child) != "" ||
			(len(child) > 0 && os.IsPathSeparator(child[0])) {
			return "", errors.New("child path must be relative to the trusted base")
		}
	}
	children := filepath.Join(paths[1:]...)
	if children != "" && !filepath.IsLocal(children) {
		return "", errors.New("joined path escaped basedir")
	}
	baseDir := filepath.Clean(paths[0])
	result = filepath.Join(baseDir, children)
	relative, err := filepath.Rel(baseDir, result)
	if err != nil {
		return "", errors.Wrap(err, "resolve path relative to trusted base")
	}
	if !filepath.IsLocal(relative) {
		return "", errors.New("joined path escaped basedir")
	}
	return result, nil
}

// IsDir is path exists as dir
func IsDir(path string) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, errors.Wrapf(err, "stat path %q", path)
	}

	return st.IsDir(), nil
}

// IsDirWritable reports whether new files can be created in dir. An empty dir
// means the current working directory. It exclusively creates an unpredictable
// private probe file, closes it and removes only that probe; existing entries,
// including links and any file named .touch, are never opened, truncated or
// removed. It returns nil when the probe was created and removed, or an error.
func IsDirWritable(dir string) (err error) {
	if dir == "" {
		dir = "."
	}
	probe, err := os.CreateTemp(dir, ".writable-probe-*")
	if err != nil {
		return errors.Wrapf(err, "create writability probe in %q", dir)
	}
	name := probe.Name()
	closeErr := probe.Close()
	if err = os.Remove(name); err != nil {
		return errors.Join(errors.Wrapf(err, "remove writability probe %q", name), closeErr)
	}
	if closeErr != nil {
		return errors.Wrapf(closeErr, "close writability probe %q", name)
	}

	return nil
}

// IsFile is path exists as file
func IsFile(path string) (bool, error) {
	isdir, err := IsDir(path)
	if err != nil {
		if os.IsNotExist(errors.Cause(err)) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	return !isdir, nil
}

// FileExists is path a valid file
//
// Returns (true, nil) if file exists and is a regular file
// Returns (false, nil) if file doesn't exist or is not a regular file
// Returns (false, error) if there's an error accessing the file
func FileExists(path string) (bool, error) {
	exists, err := IsFile(path)
	if err != nil {
		if os.IsNotExist(errors.Cause(err)) {
			return false, nil
		}
		return false, errors.Wrapf(err, "check file %q", path)
	}

	return exists, nil
}

type copyFileOption struct {
	mode      fs.FileMode
	flag      int
	overwrite bool
}

// fillDefault sets the CopyFile defaults on o: file mode 0640 and open flags os.O_WRONLY|os.O_CREATE.
// It does not modify the overwrite field, and it returns o itself so the call can be chained with
// applyOpts.
func (o *copyFileOption) fillDefault() *copyFileOption {
	o.mode = 0640
	o.flag = os.O_WRONLY | os.O_CREATE
	return o
}

// applyOpts applies each CopyFileOptionFunc in optfs to o in order. It returns o on success; when an
// option fails, it stops and returns nil with that error wrapped by the option function's name.
func (o *copyFileOption) applyOpts(optfs ...CopyFileOptionFunc) (*copyFileOption, error) {
	for _, f := range optfs {
		if err := f(o); err != nil {
			return nil, errors.Wrap(err, GetFuncName(f))
		}
	}

	return o, nil
}

// CopyFileOptionFunc set options for copy file
type CopyFileOptionFunc func(o *copyFileOption) error

// WithFileMode sets the creation mode, before the umask, of the file CopyFile
// creates. With Overwrite the replacement file also receives this mode; the
// previous destination's mode is not preserved.
func WithFileMode(perm fs.FileMode) CopyFileOptionFunc {
	return func(o *copyFileOption) error {
		o.mode = perm
		return nil
	}
}

// WithFileFlag adds open flags, such as os.O_SYNC, for the file CopyFile creates.
// Creation is always exclusive (os.O_CREATE|os.O_EXCL) and os.O_TRUNC is ignored,
// because CopyFile never opens an existing destination for writing.
func WithFileFlag(flag int) CopyFileOptionFunc {
	return func(o *copyFileOption) error {
		o.flag |= flag
		return nil
	}
}

// Overwrite allows CopyFile to replace an existing regular destination file.
// The content is written to a private temporary file beside dst and published
// with rename, so the destination entry is replaced rather than written through.
// A destination that exists but is not a regular file, such as a symbolic link,
// directory or FIFO, is rejected and left unchanged.
func Overwrite() CopyFileOptionFunc {
	return func(o *copyFileOption) error {
		o.overwrite = true
		o.flag |= os.O_TRUNC
		return nil
	}
}

// CopyFile copies the content of src to dst, creating dst's parent directories.
//
// Without Overwrite, dst is created atomically with exclusive-create semantics:
// any existing entry, including a symbolic link whose target is missing, is a
// collision and produces an error matching fs.ErrExist. With Overwrite, an
// existing regular dst is replaced through a temporary file and rename; links and
// special files are rejected. A failed copy never leaves a partial destination.
// It returns nil on success or an error describing the first failure.
func CopyFile(src, dst string, optfs ...CopyFileOptionFunc) (err error) {
	opt, err := new(copyFileOption).fillDefault().applyOpts(optfs...)
	if err != nil {
		return errors.Wrap(err, "apply options")
	}

	if err = os.MkdirAll(filepath.Dir(dst), 0751); err != nil {
		return errors.Wrapf(err, "create dir `%s`", dst)
	}

	srcFp, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "open file `%s`", src)
	}
	defer SilentClose(srcFp)

	var n int64
	copyInto := func(dstFp *os.File) error {
		var copyErr error
		if n, copyErr = io.Copy(dstFp, srcFp); copyErr != nil {
			return errors.Wrap(copyErr, "copy file")
		}
		return nil
	}
	if opt.overwrite {
		err = fileguard.Replace(dst, opt.flag, opt.mode, copyInto)
	} else {
		err = fileguard.WriteNew(dst, opt.flag, opt.mode, copyInto)
	}
	if err != nil {
		return errors.Wrapf(err, "write destination %q", dst)
	}

	log.Shared.Debug("file copied",
		zap.String("src", src),
		zap.String("dst", dst),
		zap.Int64("len", n))
	return nil
}

// IsFileATimeChanged check is file's atime equal to expectATime
func IsFileATimeChanged(path string, expectATime time.Time) (changed bool, newATime time.Time, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, time.Time{}, errors.Wrapf(err, "get stat of file %s", path)
	}

	return !fi.ModTime().Equal(expectATime), fi.ModTime(), nil
}

// FileMD5 read file and calculate MD5
//
// Deprecated: use Hash instead
func FileMD5(path string) (hashed string, err error) {
	hasher := md5.New()
	fp, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "open file %s", path)
	}

	if _, err = io.Copy(hasher, fp); err != nil {
		return "", errors.Wrapf(err, "read file %s", path)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// FileSHA1 read file and calculate sha1
//
// return hashed string in 40 bytes
//
// Deprecated: use Hash instead
func FileSHA1(path string) (hashed string, err error) {
	hasher := sha1.New()
	fp, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "open file %s", path)
	}

	if _, err = io.Copy(hasher, fp); err != nil {
		return "", errors.Wrapf(err, "read file %s", path)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// NewTmpFileForContent write content to tmp file and return path
//
// deprecated: use NewTmpFileForReader instead
func NewTmpFileForContent(content []byte) (path string, err error) {
	tmpFile, err := os.CreateTemp("", "*")
	if err != nil {
		return "", errors.Wrap(err, "create tmp file")
	}
	defer SilentClose(tmpFile)

	if _, err = tmpFile.Write(content); err != nil {
		return "", errors.Wrap(err, "write to tmp file")
	}

	return tmpFile.Name(), nil
}

// NewTmpFile write content to tmp file and return path
func NewTmpFile(reader io.Reader) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "NewTmpFileForReader-*")
	if err != nil {
		return nil, errors.Wrap(err, "create tmp file")
	}

	if _, err = io.Copy(tmpFile, reader); err != nil {
		return nil, errors.Wrapf(err, "write to tmp file %s", tmpFile.Name())
	}

	if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Wrapf(err, "seek to start of tmp file %s", tmpFile.Name())
	}

	return tmpFile, nil
}

// WatchFileChanging watch file changing
//
// when file changed, callback will be called,
// callback will only received fsnotify.Write no matter what happened to changing a file.
//
// TODO: only calculate hash when file's folder got fsnotiy
func WatchFileChanging(ctx context.Context, files []string, callback func(fsnotify.Event)) error {
	hashes := map[string]string{}
	for _, f := range files {
		hashed, err := FileSHA1(f)
		if err != nil {
			return errors.Wrapf(err, "calculate md5 for file %s", f)
		}

		hashes[f] = hashed
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			for f, hashed := range hashes {
				newHashed, err := FileSHA1(f)
				if err != nil {
					continue
				}

				if newHashed != hashed {
					hashes[f] = newHashed
					callback(fsnotify.Event{
						Name: f,
						Op:   fsnotify.Write,
					})
				}
			}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// RenderTemplate render template with args
func RenderTemplate(tplContent string, args any) ([]byte, error) {
	tpl, err := template.New("gutils").Parse(tplContent)
	if err != nil {
		return nil, errors.Wrap(err, "parse template")
	}

	var out bytes.Buffer
	if err := tpl.Execute(&out, args); err != nil {
		return nil, errors.Wrap(err, "execute with args")
	}

	return out.Bytes(), nil
}

// RenderTemplateFile render template file with args
func RenderTemplateFile(tplFile string, args any) ([]byte, error) {
	cnt, err := os.ReadFile(tplFile)
	if err != nil {
		return nil, errors.Wrapf(err, "read template file %q", tplFile)
	}

	return RenderTemplate(string(cnt), args)
}
