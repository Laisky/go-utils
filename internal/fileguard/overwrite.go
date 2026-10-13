package fileguard

import (
	"io/fs"
	"os"

	"github.com/Laisky/errors/v2"
)

// OverwriteInPlace writes an existing regular destination inode without replacing
// its metadata or hardlinks. It opens without truncation, verifies the descriptor
// against the inspected identity and source, then applies the requested truncation.
// Missing destinations are created exclusively. Final symlinks and special files
// are rejected; Unix also uses O_NOFOLLOW and O_NONBLOCK. Parent directories must
// be trusted. A write error may leave partial content because this API is in-place.
func OverwriteInPlace(path string, flag int, perm os.FileMode, source os.FileInfo,
	write func(*os.File) error) (retErr error) {
	expected, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return WriteNew(path, flag, perm, write)
	}
	if err != nil {
		return errors.Wrap(err, "inspect overwrite destination")
	}
	if !expected.Mode().IsRegular() {
		return errors.Wrap(ErrNotRegular, "overwrite destination")
	}
	// Never truncate or create while opening an inspected existing path.
	openFlag := writeAccess(flag) &^ (os.O_TRUNC | os.O_CREATE | os.O_EXCL)
	f, err := os.OpenFile(path, openFlag|appendFlags(), perm)
	if err != nil {
		return errors.Wrap(err, "open overwrite destination")
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			retErr = errors.Join(retErr, errors.Wrap(closeErr, "close overwrite destination"))
		}
	}()
	actual, err := f.Stat()
	if err != nil {
		return errors.Wrap(err, "inspect opened overwrite destination")
	}
	if !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		return errors.Wrap(ErrNotRegular, "overwrite destination changed during open")
	}
	if source != nil && os.SameFile(source, actual) {
		return errors.New("source and destination refer to the same file")
	}
	if flag&os.O_TRUNC != 0 {
		if err := f.Truncate(0); err != nil {
			return errors.Wrap(err, "truncate verified overwrite destination")
		}
	}
	if err := write(f); err != nil {
		return errors.Wrap(err, "write overwrite destination")
	}
	return nil
}
