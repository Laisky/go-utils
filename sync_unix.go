//go:build !windows
// +build !windows

package utils

import (
	"io"
	"syscall"

	"github.com/Laisky/errors/v2"
)

// FLock lock by file
type FLock interface {
	Lock() error
	Unlock() error
}

type flock struct {
	fpath string
	fd    int
}

// NewFlock new file lock
func NewFlock(lockFilePath string) FLock {
	return &flock{
		fpath: lockFilePath,
	}
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

	flock := syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: io.SeekStart,
		Start:  0,
		Len:    0,
	}
	if err := syscall.FcntlFlock(uintptr(f.fd), syscall.F_SETLK, &flock); err != nil {
		return errors.Wrap(err, "FcntlFlock(F_SETLK)")
	}

	return nil
}
