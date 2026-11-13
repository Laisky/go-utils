//go:build linux

package transactionlocal

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
)

// removeOwnedSnapshot removes only a caller-proven private staging or retired
// snapshot. It must never be used to classify ownership from an entry's name.
// Read-only directories gain temporary owner access after the snapshot is no
// longer visible; symbolic links are not traversed or chmodded.
func removeOwnedSnapshot(root string) (result error) {
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "stat owned snapshot")
	}
	if err = validateControlEntry(info, "owned snapshot", true); err != nil {
		return err
	}
	scoped, err := os.OpenRoot(root)
	if err != nil {
		return errors.Wrap(err, "open owned snapshot")
	}
	defer func() { result = errors.Join(result, errors.Wrap(scoped.Close(), "close owned snapshot")) }()
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.Wrap(walkErr, "walk owned snapshot")
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return errors.Wrap(err, "stat owned snapshot directory")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return errors.Wrap(err, "resolve owned snapshot directory")
		}
		return errors.Wrap(scoped.Chmod(relative, info.Mode().Perm()|0o700), "enable owned snapshot cleanup")
	})
	if err != nil {
		return errors.Wrap(err, "prepare owned snapshot removal")
	}
	return errors.Wrap(os.RemoveAll(root), "remove retired or aborted snapshot")
}
