//go:build linux

package transactionlocal

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/Laisky/errors/v2"
)

// validateRoot checks the namespace protecting transaction control entries.
// Only the effective user may own the root. Root and ancestor write access is
// restricted to that user and root, except trusted sticky ancestors such as /tmp.
// Read/traverse bits and caller-selected payload permissions are unchanged.
func validateRoot(root string, expected os.FileInfo) (os.FileInfo, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, errors.Wrap(err, "stat transaction root")
	}
	if !info.IsDir() {
		return nil, errors.New("transaction root must be a directory, not a symlink")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("transaction root owner is unavailable")
	}
	if int(stat.Uid) != os.Geteuid() {
		return nil, errors.New("transaction root owner must be the effective user")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("transaction root is group/world writable")
	}
	if expected != nil && !os.SameFile(expected, info) {
		return nil, errors.New("transaction root identity changed")
	}
	for ancestor := filepath.Dir(root); ; ancestor = filepath.Dir(ancestor) {
		parent, err := os.Lstat(ancestor)
		if err != nil {
			return nil, errors.Wrap(err, "stat transaction root ancestor")
		}
		if !parent.IsDir() {
			return nil, errors.New("transaction root ancestor must be a directory, not a symlink")
		}
		parentStat, ok := parent.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, errors.New("transaction root ancestor owner is unavailable")
		}
		if parentStat.Uid != 0 && int(parentStat.Uid) != os.Geteuid() {
			return nil, errors.New("transaction root ancestor owner is untrusted")
		}
		if parent.Mode().Perm()&0o022 != 0 && parent.Mode()&os.ModeSticky == 0 {
			return nil, errors.New("transaction root ancestor is group/world writable without sticky protection")
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	return info, nil
}

// validateLock checks that the opened lock is a single-link regular file owned
// by the effective user, with no untrusted write permission or path substitution.
func (engine *Engine) validateLock(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return errors.Wrap(err, "stat transaction lock")
	}
	if !info.Mode().IsRegular() {
		return errors.New("transaction lock must be a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("transaction lock owner is unavailable")
	}
	if int(stat.Uid) != os.Geteuid() {
		return errors.New("transaction lock owner must be the effective user")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return errors.New("transaction lock is group/world writable")
	}
	if stat.Nlink != 1 {
		return errors.New("transaction lock must have exactly one link")
	}
	linked, err := os.Lstat(filepath.Join(engine.root, "lock"))
	if err != nil {
		return errors.Wrap(err, "stat transaction lock path")
	}
	if !os.SameFile(info, linked) {
		return errors.New("transaction lock identity changed")
	}
	return nil
}

// validateControlEntry verifies ownership and permissions of a pointer or live
// generation entry without changing caller-selected payload file permissions.
func validateControlEntry(info os.FileInfo, label string, directory bool) error {
	if directory {
		if !info.IsDir() {
			return errors.Errorf("%s must be a directory, not a symlink", label)
		}
	} else if !info.Mode().IsRegular() {
		return errors.Errorf("%s must be a regular file, not a symlink", label)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.Errorf("%s owner is unavailable", label)
	}
	if int(stat.Uid) != os.Geteuid() {
		return errors.Errorf("%s owner must be the effective user", label)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return errors.Errorf("%s is group/world writable", label)
	}
	if !directory && stat.Nlink != 1 {
		return errors.Errorf("%s must have exactly one link", label)
	}
	return nil
}
