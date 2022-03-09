//go:build !windows
// +build !windows

package utils

import (
	"io"
	"syscall"

	"github.com/pkg/errors"
)

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
