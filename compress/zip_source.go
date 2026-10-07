package compress

import (
	"archive/zip"
	"crypto/rand"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/internal/fileguard"
)

const (
	maxZIPSourceDepth   = 256
	maxZIPSourceEntries = 100000
)

// ZipFiles archives regular files and real directories without dereferencing
// source symlinks. It publishes a private temporary archive only after all input
// reads, ZIP finalization and closes succeed; a failed archive preserves output.
// The output parent and the parents of explicitly selected inputs must be trusted.
func ZipFiles(output string, files []string) (retErr error) {
	if output == "" {
		return errors.New("ZIP output must not be empty")
	}
	output = filepath.Clean(output)
	root, err := fileguard.OpenRoot(filepath.Dir(output))
	if err != nil {
		return errors.Wrap(err, "open ZIP output parent")
	}
	defer func() { retErr = errors.Join(retErr, wrapZIPClose(root.Close(), "close ZIP output parent")) }()
	name := filepath.Base(output)
	if name == "." || name == ".." {
		return errors.New("ZIP output must name a file")
	}
	old, err := root.Lstat(name)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "inspect ZIP output")
	}
	if err == nil && !old.Mode().IsRegular() {
		return errors.New("ZIP output must not be a link or special file")
	}
	temporary := ".go-utils-zip-" + rand.Text()
	f, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Wrap(err, "create private ZIP output")
	}
	closed, published := false, false
	defer func() {
		if !closed {
			retErr = errors.Join(retErr, wrapZIPClose(f.Close(), "close incomplete ZIP output"))
		}
		if !published {
			retErr = errors.Join(retErr, wrapZIPClose(root.Remove(temporary), "remove incomplete ZIP output"))
		}
	}()
	staging, err := f.Stat()
	if err != nil {
		return errors.Wrap(err, "inspect ZIP staging file")
	}
	writer := zip.NewWriter(f)
	state := zipSourceState{writer: writer, excluded: []os.FileInfo{old, staging}}
	for _, name := range files {
		if err := state.addSelected(name, ""); err != nil {
			return errors.Wrap(err, "add selected ZIP source")
		}
	}
	if err := writer.Close(); err != nil {
		return errors.Wrap(err, "finalize ZIP output")
	}
	if err := f.Sync(); err != nil {
		return errors.Wrap(err, "sync ZIP output")
	}
	closed = true
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close ZIP output")
	}
	if err := root.Rename(temporary, name); err != nil {
		return errors.Wrap(err, "publish ZIP output")
	}
	published = true
	return nil
}

// AddFileToZip adds one selected source under a portable archive prefix. Symlinks
// and special files are rejected. On failure, the caller must discard its writer:
// already-added entries cannot be rolled back through archive/zip's API.
func AddFileToZip(writer *zip.Writer, filename, basedir string) error {
	if writer == nil {
		return errors.New("ZIP writer must not be nil")
	}
	state := zipSourceState{writer: writer}
	return errors.Wrap(state.addSelected(filename, basedir), "add ZIP source")
}

// zipSourceState tracks finite traversal and output identities across all inputs.
type zipSourceState struct {
	writer   *zip.Writer
	entries  int
	excluded []os.FileInfo
}

// addSelected anchors the selected object's parent and inspects its final component.
func (s *zipSourceState) addSelected(filename, prefix string) (retErr error) {
	if filename == "" {
		return errors.New("ZIP source must not be empty")
	}
	var err error
	filename, err = filepath.Abs(filename)
	if err != nil {
		return errors.Wrap(err, "resolve selected source")
	}
	if prefix == "." {
		prefix = ""
	}
	if prefix != "" {
		prefix = filepath.ToSlash(prefix)
		if err := validateZIPMemberName(prefix); err != nil {
			return errors.Wrap(err, "invalid ZIP prefix")
		}
	}
	parent, err := fileguard.OpenRoot(filepath.Dir(filename))
	if err != nil {
		return errors.Wrap(err, "open selected source parent")
	}
	defer func() { retErr = errors.Join(retErr, wrapZIPClose(parent.Close(), "close selected source parent")) }()
	base := filepath.Base(filename)
	info, err := parent.Lstat(base)
	if err != nil {
		return errors.Wrap(err, "inspect selected ZIP source")
	}
	return s.add(parent, base, prefix, info, 0)
}

// add walks through directory handles and verifies opened identities before reads.
func (s *zipSourceState) add(root *os.Root, name, prefix string, info os.FileInfo, depth int) (retErr error) {
	if depth > maxZIPSourceDepth || s.entries >= maxZIPSourceEntries {
		return errors.New("ZIP source traversal limit exceeded")
	}
	s.entries++
	for _, excluded := range s.excluded {
		if excluded != nil && os.SameFile(info, excluded) {
			return errors.New("ZIP source overlaps its output")
		}
	}
	member := path.Join(prefix, info.Name())
	if err := validateZIPMemberName(member); err != nil {
		return errors.Wrap(err, "invalid source member name")
	}
	if info.IsDir() {
		child, err := fileguard.OpenDirectory(root, name, info)
		if err != nil {
			return errors.Wrap(err, "open ZIP source directory")
		}
		defer func() { retErr = errors.Join(retErr, wrapZIPClose(child.Close(), "close ZIP source directory")) }()
		dir, err := child.Open(".")
		if err != nil {
			return errors.Wrap(err, "enumerate ZIP source directory")
		}
		defer func() { retErr = errors.Join(retErr, wrapZIPClose(dir.Close(), "close ZIP directory enumerator")) }()
		for {
			entries, readErr := dir.ReadDir(128)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return errors.Wrap(readErr, "read ZIP source directory")
			}
			for _, entry := range entries {
				// DirEntry.Info may resolve against a pathname after a rename.
				// Lstat through the anchored root is the source of truth instead.
				fi, err := child.Lstat(entry.Name())
				if err != nil {
					return errors.Wrap(err, "inspect ZIP directory entry")
				}
				if err := s.add(child, entry.Name(), member, fi, depth+1); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
		}
	}
	f, err := fileguard.OpenRegular(root, name, info)
	if err != nil {
		return errors.Wrap(err, "open ZIP source file")
	}
	defer func() { retErr = errors.Join(retErr, wrapZIPClose(f.Close(), "close ZIP source file")) }()
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return errors.Wrap(err, "create ZIP source header")
	}
	header.Name, header.Method = member, zip.Deflate
	output, err := s.writer.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "create ZIP member")
	}
	// Copy only the inspected size: continuously growing files cannot extend
	// this read indefinitely. A changed size or mtime invalidates the archive.
	if _, err := io.CopyN(output, f, info.Size()); err != nil {
		return errors.Wrap(err, "read ZIP source contents")
	}
	final, err := f.Stat()
	if err != nil {
		return errors.Wrap(err, "recheck ZIP source file")
	}
	if final.Size() != info.Size() || !final.ModTime().Equal(info.ModTime()) {
		return errors.New("ZIP source changed during reading")
	}
	return nil
}

// wrapZIPClose attaches context to cleanup failures without manufacturing an error on success.
func wrapZIPClose(err error, message string) error { return errors.Wrap(err, message) }
