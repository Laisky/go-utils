package utils

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/fsnotify/fsnotify"

	"github.com/Laisky/go-utils/v4/log"
)

// ReplaceFile replace file with content atomatically
//
// this function is not goroutine-safe
func ReplaceFile(path string, content []byte, perm os.FileMode) error {
	dir, fname := filepath.Split(path)
	swapFname := fmt.Sprintf(".%s.swp-%s", fname, RandomStringWithLength(6))
	swapFpath, err := JoinFilepath(dir, swapFname)
	if err != nil {
		return errors.Wrapf(err, "join path %q and %q", dir, swapFname)
	}

	fp, err := os.OpenFile(swapFpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, perm)
	if err != nil {
		return errors.Wrapf(err, "create swap file %q", swapFpath)
	}
	defer os.Remove(swapFpath) //nolint: errcheck
	defer LogErr(fp.Close, log.Shared)

	if _, err = fp.Write(content); err != nil {
		return errors.Wrapf(err, "write to file %q", swapFpath)
	}

	if err = os.Rename(swapFpath, path); err != nil {
		return errors.Wrapf(err, "replace %q by %q", path, swapFpath)
	}

	return nil
}

// JoinFilepath join paths and check if result is escaped basedir
//
// basedir is the first nonempty path in paths.
// this function could be used to prevent path escaping,
// make sure the result is under basedir.
// for example defend zip-slip: https://snyk.io/research/zip-slip-vulnerability#go
//
// Notice: cannot deal with symlink
func JoinFilepath(paths ...string) (result string, err error) {
	if len(paths) == 0 {
		return "", errors.New("empty paths")
	}

	if len(paths) == 1 {
		return paths[0], nil
	}

	for i := range paths {
		if paths[i] != "" {
			paths = paths[i:]
			break
		}
	}

	baseDir := strings.TrimRight(paths[0], string(os.PathSeparator))
	result = filepath.Clean(filepath.Join(paths...))
	if !strings.HasPrefix(result+string(os.PathSeparator), baseDir+string(os.PathSeparator)) {
		return result, errors.Errorf("got result %q, escaped basedir %q", result, baseDir)
	}

	return result, nil
}

// ReplaceFileStream replace file with content atomatically
//
// Deprecated: use ReplaceFileAtomic instead
var ReplaceFileStream = ReplaceFileAtomic

// ReplaceFileAtomic replace file with content atomatically
//
// write content to a tmp file, then rename it to dst file.
//
// Notice: this function is not goroutine-safe
func ReplaceFileAtomic(path string, in io.ReadCloser, perm os.FileMode) error {
	dir, fname := filepath.Split(path)
	swapFname := fmt.Sprintf(".%s.swp-%s", fname, RandomStringWithLength(6))
	swapFpath, err := JoinFilepath(dir, swapFname)
	if err != nil {
		return errors.Wrapf(err, "join path %q and %q", dir, swapFname)
	}

	fp, err := os.OpenFile(swapFpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, perm)
	if err != nil {
		return errors.Wrapf(err, "create swap file %q", swapFpath)
	}
	defer LogErr(func() error { return errors.Wrapf(os.Remove(swapFpath), "remove %q", swapFpath) }, log.Shared)
	defer LogErr(fp.Close, log.Shared)

	if _, err = io.Copy(fp, in); err != nil {
		return errors.Wrapf(err, "write to file %q", swapFpath)
	}

	if err = os.Rename(swapFpath, path); err != nil {
		return errors.Wrapf(err, "replace %q by %q", path, swapFpath)
	}

	return nil
}

// MoveFile move file from src to dst by copy
//
// sometimes move file by `rename` not work.
// for example, you can not move file between docker volumes by `rename`.
func MoveFile(src, dst string) (err error) {
	if err = CopyFile(src, dst); err != nil {
		return err
	}

	if err = os.Remove(src); err != nil {
		return errors.Wrapf(err, "remove file `%s`", src)
	}

	return nil
}

// IsDir is path exists as dir
func IsDir(path string) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return st.IsDir(), nil
}

// IsDirWritable if dir is writable
func IsDirWritable(dir string) (err error) {
	f := filepath.Join(dir, ".touch")
	if err = os.WriteFile(f, []byte(""), 0600); err != nil {
		return err
	}

	if err = os.Remove(f); err != nil {
		return errors.Wrapf(err, "remove file `%s`", f)
	}

	return nil
}

// IsFile is path exists as file
func IsFile(path string) (bool, error) {
	isdir, err := IsDir(path)
	return !isdir, err
}

// FileExists is path a valid file
func FileExists(path string) (bool, error) {
	ok, err := IsFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, errors.Wrapf(err, "check file %q", path)
	}

	return ok, nil
}

type copyFileOption struct {
	mode      fs.FileMode
	flag      int
	overwrite bool
}

func (o *copyFileOption) fillDefault() *copyFileOption {
	o.mode = 0640
	o.flag = os.O_WRONLY | os.O_CREATE
	return o
}

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

// WithFileMode if create new dst file, set the file's mode
func WithFileMode(perm fs.FileMode) CopyFileOptionFunc {
	return func(o *copyFileOption) error {
		o.mode = perm
		return nil
	}
}

// WithFileFlag how to write dst file
func WithFileFlag(flag int) CopyFileOptionFunc {
	return func(o *copyFileOption) error {
		o.flag |= flag
		return nil
	}
}

// Overwrite overwrite file if target existed
func Overwrite() CopyFileOptionFunc {
	return func(o *copyFileOption) error {
		o.overwrite = true
		o.flag |= os.O_TRUNC
		return nil
	}
}

// CopyFile copy file content from src to dst
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

	if !opt.overwrite {
		if ok, err := FileExists(dst); err != nil {
			return errors.Wrapf(err, "check file %q", dst)
		} else if ok {
			return errors.Errorf("file %q exists", dst)
		}
	}

	dstFp, err := os.OpenFile(dst, opt.flag, opt.mode)
	if err != nil {
		return errors.Wrapf(err, "open file `%s`", dst)
	}
	defer SilentClose(dstFp)

	var n int64
	if n, err = io.Copy(dstFp, srcFp); err != nil {
		return errors.Wrap(err, "copy file")
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

// DirSize calculate directory size
//
// inspired by https://stackoverflow.com/a/32482941/2368737
func DirSize(path string) (size int64, err error) {
	err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})

	return
}

type listFilesInDirOption struct {
	recur  bool
	filter func(fname string) bool
}

func (o *listFilesInDirOption) applyOpts(opts ...ListFilesInDirOptionFunc) (*listFilesInDirOption, error) {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// ListFilesInDirOptionFunc options for ListFilesInDir
type ListFilesInDirOptionFunc func(*listFilesInDirOption) error

// Recursive list files recursively
//
// Deprecated: use ListFilesInDirRecursive instead
func Recursive() ListFilesInDirOptionFunc {
	return func(o *listFilesInDirOption) error {
		o.recur = true
		return nil
	}
}

// ListFilesInDirRecursive list files in dir recursively
func ListFilesInDirRecursive() ListFilesInDirOptionFunc {
	return func(o *listFilesInDirOption) error {
		o.recur = true
		return nil
	}
}

// ListFilesInDirFilter filter files, only return files that filter returns true
func ListFilesInDirFilter(filter func(fname string) bool) ListFilesInDirOptionFunc {
	return func(o *listFilesInDirOption) error {
		o.filter = filter
		return nil
	}
}

// ListFilesInDir list files in dir
func ListFilesInDir(dir string, optfs ...ListFilesInDirOptionFunc) (files []string, err error) {
	log.Shared.Debug("ListFilesInDir", zap.String("dir", dir))
	opt, err := new(listFilesInDirOption).applyOpts(optfs...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	fs, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "read dir `%s`", dir)
	}

	for _, f := range fs {
		fpath, err := JoinFilepath(dir, f.Name())
		if err != nil {
			return nil, errors.Wrapf(err, "join path %q and %q", dir, f.Name())
		}

		if f.IsDir() {
			if opt.recur {
				fs, err := ListFilesInDir(fpath, optfs...)
				if err != nil {
					return nil, errors.Wrapf(err, "list files in %q", fpath)
				}

				files = append(files, fs...)
			}

			continue
		}

		if opt.filter != nil && !opt.filter(fpath) {
			continue
		}

		files = append(files, fpath)
	}

	return
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
