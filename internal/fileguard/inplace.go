package fileguard

import (
	"io/fs"
	"os"

	"github.com/Laisky/errors/v2"
)

// WriteInPlace creates path exclusively or opens its existing regular inode for
// a compatibility write. It never truncates before descriptor validation. The
// callback owns truncation and content changes; failed writes may leave partial
// contents in existing files. Hardlinks are deliberately retained, not rejected.
// On Unix O_NOFOLLOW/O_NONBLOCK prevent raced links/FIFOs; all platforms compare
// descriptor metadata before writing. Parent directories must be trusted.
func WriteInPlace(path string, flag int, perm os.FileMode, write func(*os.File) error) (retErr error) {
	f, err := CreateExclusive(path, flag|appendFlags(), perm)
	if err == nil {
		return finishWrite(f, path, path, write, nil)
	}
	if !errors.Is(err, fs.ErrExist) || flag&os.O_EXCL != 0 {
		return errors.WithStack(err)
	}
	expected, err := os.Lstat(path)
	if err != nil {
		return errors.Wrap(err, "inspect in-place destination")
	}
	if !expected.Mode().IsRegular() {
		return errors.WithStack(ErrNotRegular)
	}
	safeFlags := writeAccess(flag)&^(os.O_CREATE|os.O_EXCL) | appendFlags()
	f, err = os.OpenFile(path, safeFlags, 0)
	if err != nil {
		return errors.Wrap(err, "open in-place destination")
	}
	defer func() {
		if err := f.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "close in-place destination"))
		}
	}()
	actual, err := f.Stat()
	if err != nil {
		return errors.Wrap(err, "inspect opened in-place destination")
	}
	if !actual.Mode().IsRegular() {
		return errors.WithStack(ErrNotRegular)
	}
	if !os.SameFile(expected, actual) {
		return errors.New("destination changed while opening")
	}
	if err := write(f); err != nil {
		return errors.Wrap(err, "write in-place destination")
	}
	return nil
}
