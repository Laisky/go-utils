package utils

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/internal/fileguard"
	"github.com/Laisky/go-utils/v6/log"
)

// CopyFile copies src into dst and creates missing parent directories. Overwrite
// retains the existing regular destination inode, mode, owner, open handles and
// hardlink relationships. WithFileMode affects only creation, not existing files.
// Final symlinks and special files are rejected; parent directories must be trusted.
// An in-place copy can expose partial content or leave it after an I/O failure.
//
// Deprecated: Use CopyFileAtomic for atomic publication into a new private inode.
// That API intentionally replaces inode/metadata and does not update other hardlinks;
// it is not a drop-in replacement when callers depend on in-place semantics.
func CopyFile(src, dst string, optfs ...CopyFileOptionFunc) (retErr error) {
	opt, err := new(copyFileOption).fillDefault().applyOpts(optfs...)
	if err != nil {
		return errors.Wrap(err, "apply options")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0751); err != nil {
		return errors.Wrap(err, "create destination directory")
	}
	source, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "open source")
	}
	defer func() {
		if err := source.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "close source"))
		}
	}()
	sourceInfo, err := source.Stat()
	if err != nil {
		return errors.Wrap(err, "inspect source")
	}
	var count int64
	write := func(target *os.File) error {
		info, err := target.Stat()
		if err != nil {
			return errors.Wrap(err, "inspect destination")
		}
		if os.SameFile(sourceInfo, info) {
			return errors.New("source and destination refer to the same file")
		}
		if opt.overwrite && opt.flag&os.O_TRUNC != 0 {
			if err := target.Truncate(0); err != nil {
				return errors.Wrap(err, "truncate destination")
			}
		}
		count, err = io.Copy(target, source)
		if err != nil {
			return errors.Wrap(err, "copy file")
		}
		return nil
	}
	if opt.overwrite {
		err = fileguard.WriteInPlace(dst, opt.flag, opt.mode, write)
	} else {
		err = fileguard.WriteNew(dst, opt.flag, opt.mode, write)
	}
	if err != nil {
		return errors.Wrap(err, "write destination")
	}
	log.Shared.Debug("file copied in place", zap.String("src", src), zap.String("dst", dst), zap.Int64("len", count))
	return nil
}
