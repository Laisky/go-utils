//go:build !windows
// +build !windows

package utils

import (
	"io"
	"syscall"

	"github.com/Laisky/errors/v2"
)

// NewFlock new file lock
func NewFlock(lockFilePath string) (FLock, error) {
	return &flock{
		fpath: lockFilePath,
	}, nil
}

func (f *flock) Unlock() error {
	if err := syscall.Close(f.fd); err != nil {
		return errors.Wrap(err, "close file")
	}

	_ = syscall.Unlink(f.fpath)
	return nil
}

func (f *flock) Lock() (err error) {
	f.fd, err = syscall.Open(f.fpath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC, 0666)
	if err != nil {
		return errors.Wrapf(err, "open `%s`", f.fpath)
	}
	if f.fd < 0 {
		return errors.Errorf("open `%s`: invalid fd %d", f.fpath, f.fd)
	}

	flock := syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: io.SeekStart,
		Start:  0,
		Len:    0,
	}
	fd := uintptr(f.fd) //nolint:gosec // fd is validated non-negative and originates from syscall.Open.
	if err := syscall.FcntlFlock(fd, syscall.F_SETLK, &flock); err != nil {
		return errors.Wrap(err, "FcntlFlock(F_SETLK)")
	}

	return nil
}
