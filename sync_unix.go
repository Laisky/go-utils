//go:build unix

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

// Unlock releases the file lock by closing the descriptor opened by Lock, which drops the process's fcntl lock,
// and then removes the lock file on a best-effort basis, ignoring any unlink failure.
// It returns a wrapped error if closing the descriptor fails, for example when Lock never opened the file.
func (f *flock) Unlock() error {
	if err := syscall.Close(f.fd); err != nil {
		return errors.Wrap(err, "close file")
	}

	_ = syscall.Unlink(f.fpath)
	return nil
}

// Lock creates (if needed) and opens the lock file at f.fpath with mode 0666 (subject to umask) and then tries to
// acquire an exclusive, non-blocking fcntl write lock (F_SETLK) over the whole file.
// The lock is an advisory POSIX record lock owned by the process, so another Lock on the same file from the same
// process also succeeds, while a lock held by another process makes this call fail immediately instead of waiting.
// It returns a wrapped error if the file cannot be opened, the descriptor is invalid, or the lock cannot be
// acquired; when acquiring the lock fails the opened descriptor is closed and f.fd is reset to -1.
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
		// Release the descriptor opened above; keeping it would leak one
		// descriptor per failed attempt.
		if closeErr := syscall.Close(f.fd); closeErr != nil {
			err = errors.Join(err, errors.Wrap(closeErr, "close lock file after failed lock"))
		}
		f.fd = -1
		return errors.Wrap(err, "FcntlFlock(F_SETLK)")
	}

	return nil
}
