package utils

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Laisky/zap"
	"github.com/pkg/errors"
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
	if err = ioutil.WriteFile(f, []byte(""), os.ModePerm); err != nil {
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
	Logger.Debug("copy file", zap.String("dst", dst), zap.Int64("len", n))

	return nil
}

// DirSize calculate directory size.
// https://stackoverflow.com/a/32482941/2368737
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
	fs, err := ioutil.ReadDir(dir)
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
