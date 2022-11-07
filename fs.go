package utils

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"
	"github.com/fsnotify/fsnotify"

	"github.com/Laisky/go-utils/v2/log"
)

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
	if err = os.WriteFile(f, []byte(""), os.ModePerm); err != nil {
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

// CopyFile copy file content from src to dst
func CopyFile(src, dst string) (err error) {
	if err = os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return errors.Wrapf(err, "create dir `%s`", dst)
	}

	srcFp, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "open file `%s`", src)
	}

	dstFp, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "open file `%s`", dst)
	}

	var n int64
	if n, err = io.Copy(dstFp, srcFp); err != nil {
		return errors.Wrap(err, "copy file")
	}
	log.Shared.Debug("copy file", zap.String("dst", dst), zap.Int64("len", n))

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
func FileMD5(path string) (hashed string, err error) {
	hasher := md5.New()
	fp, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "open file %s", path)
	}

	chunk := make([]byte, 4096)
	for {
		n, err := fp.Read(chunk)
		if err != nil {
			if err == io.EOF {
				break
			}

			return "", errors.Wrapf(err, "read file %s", path)
		}

		// log.Shared.Info("md5 read",
		// 	zap.String("file", path),
		// 	zap.ByteString("cnt", chunk[:n]))
		hasher.Write(chunk[:n])
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// FileSHA1 read file and calculate sha1
//
// return hashed string in 40 bytes
func FileSHA1(path string) (hashed string, err error) {
	hasher := sha1.New()
	fp, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "open file %s", path)
	}

	chunk := make([]byte, 4096)
	for {
		n, err := fp.Read(chunk)
		if err != nil {
			if err == io.EOF {
				break
			}

			return "", errors.Wrapf(err, "read file %s", path)
		}

		// log.Shared.Info("sha1 read",
		// 	zap.String("file", path),
		// 	zap.ByteString("cnt", chunk[:n]))
		hasher.Write(chunk[:n])
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

// ListFilesInDir list files in dir
func ListFilesInDir(dir string) (files []string, err error) {
	fs, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "read dir `%s`", dir)
	}

	for _, f := range fs {
		if f.IsDir() {
			continue
		}

		files = append(files, filepath.Join(dir, f.Name()))
	}

	return
}

// NewTmpFileForContent write content to tmp file and return path
func NewTmpFileForContent(content []byte) (path string, err error) {
	tmpFile, err := os.CreateTemp("", "*")
	if err != nil {
		return "", errors.Wrap(err, "create tmp file")
	}
	defer CloseQuietly(tmpFile)

	if _, err = tmpFile.Write(content); err != nil {
		return "", errors.Wrap(err, "write to tmp file")
	}

	return tmpFile.Name(), nil
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

// WatchFileChangingByMtime watch file changing
//
// when file changed, callback will be called,
// callback will only received fsnotify.Write no matter what happened to changing a file.
//
// BUG: Mtime is only accurate to the second
//
// Deprecated: use WatchFileChanging instead
func WatchFileChangingByMtime(ctx context.Context, files []string, callback func(fsnotify.Event)) error {
	atimes := map[string]time.Time{}
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			return errors.Wrapf(err, "get stat of file %s", f)
		}

		atimes[f] = fi.ModTime()
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}

			for f, atime := range atimes {
				changed, atime, err := IsFileATimeChanged(f, atime)
				if err != nil {
					continue
				}

				atimes[f] = atime
				if changed {
					callback(fsnotify.Event{
						Name: f,
						Op:   fsnotify.Write,
					})
				}
			}
		}
	}()

	return nil
}

// WatchFileChangingByNotify watch file changing
//
// when file changed, callback will be called
//
// BUG: Tools like vim will delete and replace files before writing,
// which will cause the notify tool to fail
//
// https://github.com/fsnotify/fsnotify/issues/255#issuecomment-407575900
//
// Deprecated: use WatchFileChanging instead
func WatchFileChangingByNotify(ctx context.Context, files []string, callback func(fsnotify.Event)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "create watcher")
	}

	for _, f := range files {
		if err = watcher.Add(f); err != nil {
			return errors.Wrapf(err, "add file `%s` to watcher", f)
		}
	}

	go func() {
		defer CloseQuietly(watcher)
		for {
			select {
			case evt := <-watcher.Events:
				if evt.Op&fsnotify.Write == fsnotify.Write {
					callback(evt)
				}
			case err := <-watcher.Errors:
				log.Shared.Error("watch file error", zap.Error(err))
			case <-ctx.Done():
				log.Shared.Debug("watcher exit", zap.Error(ctx.Err()))
				return
			}
		}
	}()

	return nil
}
