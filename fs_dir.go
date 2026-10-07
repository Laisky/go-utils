package utils

import (
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

// DirSize calculate directory size
//
// inspired by https://stackoverflow.com/a/32482941/2368737
func DirSize(path string) (size int64, err error) {
	err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return size, errors.Wrapf(err, "walk directory %q", path)
	}

	return
}

type listFilesInDirOption struct {
	recur  bool
	filter func(fname string) bool
}

// applyOpts applies each ListFilesInDirOptionFunc in opts to o in order. It returns o on success; when
// an option fails, it stops and returns nil with that error wrapped as "apply option".
func (o *listFilesInDirOption) applyOpts(opts ...ListFilesInDirOptionFunc) (*listFilesInDirOption, error) {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, errors.Wrap(err, "apply option")
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
