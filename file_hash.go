package utils

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"hash"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/internal/fileguard"
)

// DefaultFileHashMaxBytes bounds file-based hashing; reader-based Hash remains streaming.
const DefaultFileHashMaxBytes int64 = 1 << 30

// ErrFileHashLimit reports rejection before or during a bounded file read.
var ErrFileHashLimit = errors.New("file exceeds hashing byte limit")

// FileHash hashes an ordinary file using a finite default byte budget. Final
// symlinks and special files are rejected; the selected parent must be trusted.
func FileHash(hashType HashTypeInterface, path string) ([]byte, error) {
	return FileHashWithContext(context.Background(), hashType, path, DefaultFileHashMaxBytes)
}

// FileHashWithContext hashes a regular file under an explicit positive byte cap.
// Zero selects DefaultFileHashMaxBytes. Cancellation closes the descriptor and is
// checked between reads; a filesystem syscall that cannot be interrupted by the
// operating system is not given a hard real-time deadline by this API.
func FileHashWithContext(ctx context.Context, hashType HashTypeInterface, path string, maxBytes int64) (signature []byte, retErr error) {
	if ctx == nil || hashType == nil {
		return nil, errors.New("hash context and algorithm must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "hash file")
	}
	if maxBytes < 0 {
		return nil, errors.New("file hashing byte limit must not be negative")
	}
	if maxBytes == 0 {
		maxBytes = DefaultFileHashMaxBytes
	}
	if path == "" {
		return nil, errors.New("file path must not be empty")
	}
	hasher, err := hashType.Hasher()
	if err != nil {
		return nil, errors.Wrap(err, "create file hasher")
	}
	if hasher == nil {
		return nil, errors.New("file hasher must not be nil")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrap(err, "resolve hash input")
	}
	root, err := fileguard.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errors.Wrap(err, "open hash input parent")
	}
	defer func() { retErr = errors.Join(retErr, errors.Wrap(root.Close(), "close hash input parent")) }()
	name := filepath.Base(path)
	expected, err := root.Lstat(name)
	if err != nil {
		return nil, errors.Wrap(err, "inspect hash input")
	}
	if !expected.Mode().IsRegular() {
		return nil, errors.New("hash input must be a regular file, not a link or special file")
	}
	if expected.Size() < 0 || expected.Size() > maxBytes {
		return nil, errors.WithStack(ErrFileHashLimit)
	}
	f, err := fileguard.OpenRegular(root, name, expected)
	if err != nil {
		return nil, errors.Wrap(err, "open hash input")
	}
	// One close owner avoids a cancellation/defer race and joins the callback.
	// No goroutine is used to hide a potentially stuck read from the caller.
	closed := make(chan struct{})
	var closeErr error
	closeFile := sync.OnceFunc(func() { closeErr = f.Close(); close(closed) })
	stop := context.AfterFunc(ctx, closeFile)
	defer func() {
		stop()
		closeFile()
		<-closed
		retErr = errors.Join(retErr, errors.Wrap(closeErr, "close hash input"))
	}()
	if err := hashBoundedFile(ctx, hasher, f, maxBytes); err != nil {
		return nil, errors.Wrap(err, "read hash input")
	}
	actual, err := f.Stat()
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "hash file")
	}
	if err != nil {
		return nil, errors.Wrap(err, "recheck hash input")
	}
	if actual.Size() != expected.Size() || !actual.ModTime().Equal(expected.ModTime()) {
		return nil, errors.New("hash input changed while reading")
	}
	return hasher.Sum(nil), nil
}

// hashBoundedFile caps bytes read and hashed even when a source grows after stat.
// A single probe byte distinguishes exact-budget EOF from an oversized source.
func hashBoundedFile(ctx context.Context, hasher hash.Hash, input io.Reader, maximum int64) error {
	var total int64
	var buffer [32 * 1024]byte
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "hash canceled")
		}
		size := len(buffer)
		if remaining := maximum - total; remaining < int64(size) {
			size = int(remaining) + 1
		}
		n, readErr := input.Read(buffer[:size])
		if n < 0 || n > size {
			return errors.New("invalid file read count")
		}
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "hash canceled")
		}
		if int64(n) > maximum-total {
			return errors.WithStack(ErrFileHashLimit)
		}
		if n != 0 {
			written, err := hasher.Write(buffer[:n])
			if err != nil {
				return errors.Wrap(err, "hash file bytes")
			}
			if written != n {
				return errors.Wrap(io.ErrShortWrite, "hash file bytes")
			}
			total += int64(n)
			emptyReads = 0
		} else {
			emptyReads++
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return errors.Wrap(readErr, "read file bytes")
		}
		if emptyReads >= 100 {
			return errors.Wrap(io.ErrNoProgress, "read file bytes")
		}
	}
}

// VerifyFileHash validates a sha256:hex or md5:hex digest with default file limits.
func VerifyFileHash(path, encoded string) error {
	return VerifyFileHashWithContext(context.Background(), path, encoded, DefaultFileHashMaxBytes)
}

// VerifyFileHashWithContext rejects malformed digests before I/O, then compares
// decoded fixed-length digests in constant time. It never prints either digest.
func VerifyFileHashWithContext(ctx context.Context, path, encoded string, maxBytes int64) error {
	if len(encoded) > len("sha256:")+64 {
		return errors.New("invalid encoded file hash length")
	}
	algorithm, text, ok := strings.Cut(encoded, ":")
	if !ok {
		return errors.New("invalid file hash format; expected algorithm:hex")
	}
	var hashType HashType
	var expectedLength int
	switch algorithm {
	case "sha256":
		hashType, expectedLength = HashTypeSha256, 32
	case "md5":
		hashType, expectedLength = HashTypeMD5, 16
	default:
		return errors.New("unsupported file hash algorithm")
	}
	if len(text) != 2*expectedLength {
		return errors.New("invalid file digest length")
	}
	expected, err := hex.DecodeString(text)
	if err != nil {
		return errors.New("invalid hexadecimal file digest")
	}
	actual, err := FileHashWithContext(ctx, hashType, path, maxBytes)
	if err != nil {
		return errors.Wrap(err, "verify file hash")
	}
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return errors.New("file hash mismatch")
	}
	return nil
}
