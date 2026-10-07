package fileguard

import (
	"crypto/rand"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
)

// ErrNotRegular reports a destination entry that exists but is not a regular file,
// such as a symbolic link, directory, FIFO or device.
var ErrNotRegular = errors.New("destination exists and is not a regular file")

// writeAccess normalizes caller-supplied open flags for a freshly created file.
// It drops O_TRUNC, which is meaningless for exclusive creation, and adds write
// access when neither O_WRONLY nor O_RDWR is present. It returns the flags.
func writeAccess(flag int) int {
	flag &^= os.O_TRUNC
	if flag&(os.O_WRONLY|os.O_RDWR) == 0 {
		flag |= os.O_WRONLY
	}
	return flag
}

// CreateExclusive atomically creates a new file at path. O_CREATE|O_EXCL refuses
// every existing entry, including final symbolic links whose target is missing,
// so the call can never write through a planted link. flag may add open flags
// such as O_APPEND or O_SYNC; perm is the creation mode before the umask. It
// returns the open file or an error that matches fs.ErrExist on a collision.
func CreateExclusive(path string, flag int, perm os.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, writeAccess(flag)|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, errors.Wrapf(err, "destination %q already exists", path)
		}
		return nil, errors.Wrapf(err, "create destination %q exclusively", path)
	}
	return f, nil
}

// WriteNew exclusively creates path, lets write fill it and closes it. When write
// or close fails, the partially written file created by this call is removed so
// no incomplete output is left behind. flag and perm are passed to
// CreateExclusive. It returns nil on success or the first failure joined with any
// cleanup failure.
func WriteNew(path string, flag int, perm os.FileMode, write func(*os.File) error) error {
	f, err := CreateExclusive(path, flag, perm)
	if err != nil {
		return err
	}
	return finishWrite(f, path, path, write, nil)
}

// Replace publishes new content at path without ever writing through an existing
// entry. An existing destination must be a regular file; links, directories and
// special files are rejected with ErrNotRegular. Content is written to a private
// exclusively created temporary file in the destination directory and published
// with rename, which replaces the directory entry and therefore never modifies a
// link target or another hard link of the previous file. The previous file's
// mode, owner and extra hard links are not carried over. flag and perm are passed
// to CreateExclusive for the temporary file. It returns nil on success or an error;
// on failure the destination is left unchanged and the temporary file is removed.
func Replace(path string, flag int, perm os.FileMode, write func(*os.File) error) error {
	if err := RequireRegularOrAbsent(path); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(path), ".gutils-"+rand.Text()+".tmp")
	f, err := CreateExclusive(temp, flag, perm)
	if err != nil {
		return errors.Wrap(err, "create private replacement file")
	}
	return finishWrite(f, temp, path, write, func() error {
		if err := os.Rename(temp, path); err != nil {
			return errors.Wrapf(err, "replace %q", path)
		}
		return nil
	})
}

// RequireRegularOrAbsent inspects path without following a final link. It returns
// nil when nothing exists at path or when the entry is a regular file, an error
// wrapping ErrNotRegular for any other entry type, or the inspection error.
func RequireRegularOrAbsent(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "inspect destination %q", path)
	}
	if !info.Mode().IsRegular() {
		return errors.Wrapf(ErrNotRegular, "destination %q has type %s", path, info.Mode().Type())
	}
	return nil
}

// finishWrite runs write on f, closes it and optionally publishes it. f was
// created at created; when any step fails before publication succeeds, created is
// removed. target names the logical destination for error messages, and publish
// may be nil when f already lives at its final path. It returns nil on success or
// the failure joined with any cleanup failure.
func finishWrite(f *os.File, created, target string, write func(*os.File) error,
	publish func() error) (retErr error) {
	closed, published := false, false
	defer func() {
		if !closed {
			if err := f.Close(); err != nil {
				retErr = errors.Join(retErr, errors.Wrapf(err, "close incomplete output for %q", target))
			}
		}
		if !published {
			if err := os.Remove(created); err != nil && !errors.Is(err, fs.ErrNotExist) {
				retErr = errors.Join(retErr, errors.Wrapf(err, "remove incomplete output for %q", target))
			}
		}
	}()
	if err := write(f); err != nil {
		return errors.Wrapf(err, "write output for %q", target)
	}
	// Windows cannot rename an open file, so close before publishing everywhere.
	closed = true
	if err := f.Close(); err != nil {
		return errors.Wrapf(err, "close output for %q", target)
	}
	if publish != nil {
		if err := publish(); err != nil {
			return err
		}
	}
	published = true
	return nil
}

// OpenAppend opens path for appending without following a final link, blocking on
// a FIFO or reusing anything other than a private regular file. A missing file is
// created atomically with perm before the umask. An existing entry is inspected
// without following links, reopened without creation and then verified through
// the opened descriptor: it must still be the same regular file, and on Unix it
// must not have additional hard links. On Unix the open also uses O_NOFOLLOW and
// O_NONBLOCK, so a raced link fails and a raced FIFO cannot block. The parent
// directory must be trusted by the caller. It returns the open file or an error.
func OpenAppend(path string, perm os.FileMode) (*os.File, error) {
	extra := appendFlags()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_EXCL|extra, perm)
	if err == nil {
		return verifyAppend(f, path, nil)
	}
	if !errors.Is(err, fs.ErrExist) {
		return nil, errors.Wrapf(err, "create append destination %q", path)
	}
	expected, err := os.Lstat(path)
	if err != nil {
		return nil, errors.Wrapf(err, "inspect append destination %q", path)
	}
	if !expected.Mode().IsRegular() {
		return nil, errors.Wrapf(ErrNotRegular, "append destination %q has type %s", path, expected.Mode().Type())
	}
	f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND|extra, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "open append destination %q", path)
	}
	return verifyAppend(f, path, expected)
}

// verifyAppend checks the opened descriptor before any byte is written. It takes
// the open file, its path for messages and the inspected metadata, which is nil
// for a file created by this call. It returns f on success; otherwise it closes f
// and returns the rejection joined with any close failure.
func verifyAppend(f *os.File, path string, expected os.FileInfo) (*os.File, error) {
	actual, err := f.Stat()
	var reason error
	switch {
	case err != nil:
		reason = errors.Wrapf(err, "inspect opened append destination %q", path)
	case !actual.Mode().IsRegular():
		reason = errors.Wrapf(ErrNotRegular, "opened append destination %q has type %s", path, actual.Mode().Type())
	case expected != nil && !os.SameFile(expected, actual):
		reason = errors.Errorf("append destination %q changed while opening", path)
	case hasExtraLinks(actual):
		reason = errors.Errorf("append destination %q has additional hard links", path)
	default:
		return f, nil
	}
	if closeErr := f.Close(); closeErr != nil {
		reason = errors.Join(reason, errors.Wrapf(closeErr, "close rejected append destination %q", path))
	}
	return nil, reason
}
