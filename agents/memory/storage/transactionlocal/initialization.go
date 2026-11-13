//go:build linux

package transactionlocal

import (
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
)

// makeRootDirectories creates missing path components with owner-only
// permissions and returns only directories created by this call, outermost first.
// Existing directories, including symlink aliases to directories, are unchanged.
func makeRootDirectories(root string) ([]string, error) {
	missing := []string{}
	for directory := root; ; directory = filepath.Dir(directory) {
		info, err := os.Stat(directory)
		if err == nil {
			if !info.IsDir() {
				return nil, errors.New("transaction root path contains a non-directory")
			}
			break
		}
		if !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "inspect transaction root path")
		}
		missing = append(missing, directory)
		if filepath.Dir(directory) == directory {
			return nil, errors.New("transaction root path has no existing ancestor")
		}
	}
	created := make([]string, 0, len(missing))
	for index := len(missing) - 1; index >= 0; index-- {
		directory := missing[index]
		if err := os.Mkdir(directory, 0o700); err != nil {
			// A cooperating initializer may have created this component meanwhile.
			// Its pre-existing directory is not one of this call's new entries.
			if !os.IsExist(err) {
				return created, errors.Wrap(err, "create transaction root directory")
			}
			info, statErr := os.Stat(directory)
			if statErr != nil {
				return created, errors.Wrap(statErr, "inspect concurrently created root directory")
			}
			if !info.IsDir() {
				return created, errors.New("transaction root path contains a non-directory")
			}
			continue
		}
		created = append(created, directory)
	}
	return created, nil
}

// syncCreatedDirectories makes this constructor's newly created directory
// entries durable, bottom-up, without opening unchanged higher ancestors.
// Each new directory and its immediate parent is synchronized at most once.
// Existing roots require no constructor synchronization; their initialization
// durability belongs to their creator. A required parent sync failure is returned.
func syncCreatedDirectories(created []string, syncDir func(string) error) error {
	synced := make(map[string]struct{}, len(created)+1)
	for index := len(created) - 1; index >= 0; index-- {
		directory := created[index]
		for _, target := range []string{directory, filepath.Dir(directory)} {
			if _, ok := synced[target]; ok {
				continue
			}
			if err := syncDir(target); err != nil {
				return errors.Wrap(err, "sync newly created transaction directory")
			}
			synced[target] = struct{}{}
		}
	}
	return nil
}

// removeCreatedDirectories discards this constructor's newly created empty
// directories after failure. It never recursively removes contents or touches
// pre-existing directories; nonempty concurrent content is retained and reported.
func removeCreatedDirectories(created []string) error {
	var result error
	for index := len(created) - 1; index >= 0; index-- {
		if err := os.Remove(created[index]); err != nil && !os.IsNotExist(err) {
			result = errors.Join(result, errors.Wrap(err, "remove uninitialized transaction directory"))
		}
	}
	return result
}
