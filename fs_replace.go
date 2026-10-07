package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

// ReplaceFile replace file with content atomatically
//
// this function is not goroutine-safe
func ReplaceFile(path string, content []byte, perm os.FileMode) error {
	dir, fname := filepath.Dir(path), filepath.Base(path)
	swapFname := fmt.Sprintf(".%s.swp-%s", fname, RandomStringWithLength(6))
	swapFpath, err := JoinFilepath(dir, swapFname)
	if err != nil {
		return errors.Wrapf(err, "join path %q and %q", dir, swapFname)
	}

	// Security: use O_EXCL so open fails if the swap path already exists. This
	// refuses to follow an attacker-planted symlink or overwrite a pre-created
	// file at the (randomized) swap path. O_TRUNC is unnecessary because O_EXCL
	// guarantees a freshly created file.
	fp, err := os.OpenFile(swapFpath, os.O_CREATE|os.O_EXCL|os.O_RDWR, perm)
	if err != nil {
		return errors.Wrapf(err, "create swap file %q", swapFpath)
	}
	defer os.Remove(swapFpath) //nolint: errcheck
	closed := false
	defer func() {
		if !closed {
			LogErr(fp.Close, log.Shared)
		}
	}()

	if _, err = fp.Write(content); err != nil {
		return errors.Wrapf(err, "write to file %q", swapFpath)
	}

	// Windows cannot rename this file while its handle is open. Close before
	// publishing on every platform and return a close failure without replacing
	// the destination. Do not defer a second close of the same handle.
	closed = true
	if err = fp.Close(); err != nil {
		return errors.Wrapf(err, "close replacement file %q", swapFpath)
	}
	if err = os.Rename(swapFpath, path); err != nil {
		return errors.Wrapf(err, "replace %q by %q", path, swapFpath)
	}

	return nil
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
	dir, fname := filepath.Dir(path), filepath.Base(path)
	swapFname := fmt.Sprintf(".%s.swp-%s", fname, RandomStringWithLength(6))
	swapFpath, err := JoinFilepath(dir, swapFname)
	if err != nil {
		return errors.Wrapf(err, "join path %q and %q", dir, swapFname)
	}

	// Security: use O_EXCL so open fails if the swap path already exists. This
	// refuses to follow an attacker-planted symlink or overwrite a pre-created
	// file at the (randomized) swap path. O_TRUNC is unnecessary because O_EXCL
	// guarantees a freshly created file.
	fp, err := os.OpenFile(swapFpath, os.O_CREATE|os.O_EXCL|os.O_RDWR, perm)
	if err != nil {
		return errors.Wrapf(err, "create swap file %q", swapFpath)
	}
	defer func() {
		if err := os.Remove(swapFpath); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Shared.Error("remove unpublished replacement file", zap.Error(err))
		}
	}()
	closed := false
	defer func() {
		if !closed {
			LogErr(fp.Close, log.Shared)
		}
	}()

	if _, err = io.Copy(fp, in); err != nil {
		return errors.Wrapf(err, "write to file %q", swapFpath)
	}

	// Windows cannot rename this file while its handle is open. Close before
	// publishing on every platform and return a close failure without replacing
	// the destination. Do not defer a second close of the same handle.
	closed = true
	if err = fp.Close(); err != nil {
		return errors.Wrapf(err, "close replacement file %q", swapFpath)
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
//
// dst must not exist: it is created exclusively, so an existing entry or a
// dangling link is a collision and src is kept. See CopyFile.
func MoveFile(src, dst string) (err error) {
	if err = CopyFile(src, dst); err != nil {
		return errors.Wrapf(err, "copy file from %q to %q", src, dst)
	}

	if err = os.Remove(src); err != nil {
		return errors.Wrapf(err, "remove file `%s`", src)
	}

	return nil
}
