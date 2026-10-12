package local

import (
	"context"
	"io"
	"os"

	"github.com/Laisky/errors/v2"
)

// readRootRange reads only the requested regular-file range through root,
// checking the byte budget before allocation and cancellation between 32 KiB
// reads. A missing file returns empty content; concurrent truncation returns
// the available prefix. offset and length must already be syntactically valid.
func readRootRange(ctx context.Context, root *os.Root, relPath string, offset, length, maxBytes int64) (content string, err error) {
	if err := ctx.Err(); err != nil {
		return "", errors.Wrap(err, "read context")
	}
	file, err := root.Open(relPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(err, "open read file")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			content = ""
			err = errors.Join(err, errors.Wrap(closeErr, "close read file"))
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return "", errors.Wrap(err, "stat read file")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("read path is not a regular file")
	}
	if offset > info.Size() {
		return "", errors.New("offset out of range")
	}
	count := info.Size() - offset
	if length >= 0 {
		count = min(count, length)
	}
	if count > maxBytes {
		return "", errors.Wrap(ErrBudgetExceeded, "read range exceeds byte limit")
	}
	if err := ctx.Err(); err != nil {
		return "", errors.Wrap(err, "read context")
	}
	body := make([]byte, int(count))
	read := 0
	for read < len(body) {
		if err := ctx.Err(); err != nil {
			return "", errors.Wrap(err, "read context")
		}
		end := read + min(32*1024, len(body)-read)
		n, readErr := file.ReadAt(body[read:end], offset+int64(read))
		read += n
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", errors.Wrap(readErr, "read file range")
		}
		if n == 0 {
			return "", errors.Wrap(io.ErrNoProgress, "read file range")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", errors.Wrap(err, "read context")
	}
	return string(body[:read]), nil
}
