package cmd

import (
	"bytes"
	"io"
	"io/fs"
	"os"

	"github.com/Laisky/errors/v2"
)

// compareChunkBytes bounds the memory used per file while comparing content.
const compareChunkBytes = 32 * 1024

// contentComparison describes the outcome of a byte-for-byte file comparison.
// first and second are the metadata of the opened descriptors that were compared.
type contentComparison struct {
	equal  bool
	first  os.FileInfo
	second os.FileInfo
}

// compareFileContent decides whether two regular files hold identical bytes. A
// digest match is only a candidate index; this helper compares sizes first and
// then streams both files in bounded chunks. It takes the two paths and returns
// the comparison with descriptor metadata, or an error when either file cannot be
// opened, is not a regular file or cannot be read. Callers must treat an error as
// "not proven identical" and preserve both files.
func compareFileContent(first, second string) (result contentComparison, retErr error) {
	firstFp, firstInfo, err := openRegularForCompare(first)
	if err != nil {
		return result, err
	}
	defer func() {
		if err := firstFp.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrapf(err, "close %q", first))
		}
	}()
	secondFp, secondInfo, err := openRegularForCompare(second)
	if err != nil {
		return result, err
	}
	defer func() {
		if err := secondFp.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrapf(err, "close %q", second))
		}
	}()

	result.first, result.second = firstInfo, secondInfo
	if firstInfo.Size() != secondInfo.Size() {
		return result, nil
	}
	equal, err := sameStreamContent(firstFp, secondFp)
	if err != nil {
		return contentComparison{}, errors.Wrapf(err, "compare %q with %q", first, second)
	}
	result.equal = equal
	return result, nil
}

// openRegularForCompare opens path for reading and verifies through the opened
// descriptor that it is a regular file. It takes the path and returns the open
// file and its descriptor metadata, or an error; nothing is left open on error.
func openRegularForCompare(path string) (*os.File, os.FileInfo, error) {
	fp, err := os.Open(path)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "open %q for comparison", path)
	}
	info, err := fp.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.Errorf("%q is not a regular file", path)
	}
	if err != nil {
		if closeErr := fp.Close(); closeErr != nil {
			err = errors.Join(err, errors.Wrapf(closeErr, "close %q", path))
		}
		return nil, nil, errors.Wrapf(err, "inspect %q for comparison", path)
	}
	return fp, info, nil
}

// sameStreamContent reads both readers in fixed-size chunks and reports whether
// they produce identical byte sequences of identical length. It takes the two
// readers and returns the verdict or the first read error.
func sameStreamContent(first, second io.Reader) (bool, error) {
	firstBuf := make([]byte, compareChunkBytes)
	secondBuf := make([]byte, compareChunkBytes)
	for {
		firstN, firstErr := io.ReadFull(first, firstBuf)
		secondN, secondErr := io.ReadFull(second, secondBuf)
		if err := chunkReadError(firstErr); err != nil {
			return false, err
		}
		if err := chunkReadError(secondErr); err != nil {
			return false, err
		}
		if firstN != secondN || !bytes.Equal(firstBuf[:firstN], secondBuf[:secondN]) {
			return false, nil
		}
		if firstErr != nil || secondErr != nil {
			// A short chunk ends a stream; both must end on the same chunk.
			return firstErr != nil && secondErr != nil, nil
		}
	}
}

// chunkReadError converts the io.ReadFull end-of-stream markers into nil and
// wraps any other read failure. It takes the read error and returns nil or a
// wrapped error.
func chunkReadError(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil
	}
	return errors.Wrap(err, "read file chunk for comparison")
}

// removeIfUnchanged removes path only when its directory entry still refers to the
// compared inode with the same size and modification time, narrowing the window
// in which a concurrently replaced file could be deleted. It takes the path and
// the descriptor metadata recorded during comparison, and returns an error when
// the entry changed or removal fails.
func removeIfUnchanged(path string, compared os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil {
		return errors.Wrapf(err, "inspect %q before removal", path)
	}
	if !current.Mode().IsRegular() || !os.SameFile(current, compared) ||
		current.Size() != compared.Size() || !current.ModTime().Equal(compared.ModTime()) {
		return errors.Errorf("%q changed after comparison; preserved", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Wrapf(err, "remove file %q", path)
	}
	return nil
}
