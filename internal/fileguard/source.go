// Package fileguard opens checked source objects through anchored directory handles
// and creates, replaces or appends to destinations without following final links.
package fileguard

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/Laisky/errors/v2"
)

// OpenRoot anchors an explicitly selected directory after rejecting a final link.
// Its parent path and mount namespace must be trusted by the caller. A replaced
// directory is rejected by opened-object identity before any contents are read.
func OpenRoot(name string) (*os.Root, error) {
	if name == "" || runtime.GOOS == "js" {
		return nil, errors.New("source root is empty or unsupported")
	}
	name = filepath.Clean(name)
	expected, err := os.Lstat(name)
	if err != nil {
		return nil, errors.Wrap(err, "inspect source directory")
	}
	if !expected.IsDir() {
		return nil, errors.New("source must be a real directory, not a link or special file")
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, errors.Wrap(err, "open source directory")
	}
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, actual) {
		closeErr := root.Close()
		return nil, errors.Join(errors.New("source directory changed while opening"), err, closeErr)
	}
	return root, nil
}

// OpenDirectory opens a child directory relative to root without trusting a
// pathname after inspection. Rooted traversal prevents outside-link races;
// descriptor identity rejects replacement before enumeration.
func OpenDirectory(root *os.Root, name string, expected os.FileInfo) (*os.Root, error) {
	if root == nil || expected == nil || !expected.IsDir() || !local(name) {
		return nil, errors.New("invalid source directory")
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return nil, errors.Wrap(err, "open rooted directory")
	}
	actual, err := child.Stat(".")
	if err != nil || !os.SameFile(expected, actual) {
		closeErr := child.Close()
		return nil, errors.Join(errors.New("source directory changed while opening"), err, closeErr)
	}
	return child, nil
}

// OpenRegular opens a previously inspected regular file without unbounded FIFO
// waits, then verifies the actual descriptor before any byte is read. Symlink
// metadata is rejected; a raced link cannot escape root or substitute an inode.
func OpenRegular(root *os.Root, name string, expected os.FileInfo) (*os.File, error) {
	if root == nil || expected == nil || !expected.Mode().IsRegular() || !local(name) {
		return nil, errors.New("source must be a regular file, not a link or special file")
	}
	flags, err := readFlags()
	if err != nil {
		return nil, errors.Wrap(err, "select safe file flags")
	}
	f, err := root.OpenFile(name, flags, 0)
	if err != nil {
		return nil, errors.Wrap(err, "open rooted regular file")
	}
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		closeErr := f.Close()
		return nil, errors.Join(errors.New("source file changed while opening"), err, closeErr)
	}
	return f, nil
}

// local requires a cleaned, confined path without trailing separators before
// rooted operations, including when compiled with older os.Root implementations.
func local(name string) bool {
	return name != "" && name == filepath.Clean(name) && filepath.IsLocal(name)
}
