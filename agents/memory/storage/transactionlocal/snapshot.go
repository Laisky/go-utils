//go:build linux

package transactionlocal

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Laisky/errors/v2"
)

// removeIfPresent removes a temporary pointer, accepting its published absence.
func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return errors.Wrap(err, "remove temporary pointer")
}

// copyTree copies regular files and directories without following symbolic links.
func copyTree(ctx context.Context, source, target string) error {
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.Wrap(walkErr, "walk generation")
		}
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "copy generation canceled")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return errors.Wrap(err, "resolve generation path")
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return errors.Wrap(err, "stat generation entry")
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return errors.Wrap(os.Mkdir(destination, info.Mode().Perm()), "copy generation directory")
		}
		if !info.Mode().IsRegular() {
			return errors.New("generation contains a non-regular file")
		}
		return copyFile(ctx, path, destination, info.Mode().Perm())
	})
	return errors.Wrap(err, "copy generation")
}

// copyFile copies a single regular file while checking cancellation per chunk.
func copyFile(ctx context.Context, source, target string, mode os.FileMode) (result error) {
	input, err := os.Open(source)
	if err != nil {
		return errors.Wrap(err, "open snapshot source")
	}
	defer func() { result = errors.Join(result, errors.Wrap(input.Close(), "close snapshot source")) }()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return errors.Wrap(err, "create snapshot file")
	}
	defer func() { result = errors.Join(result, errors.Wrap(output.Close(), "close snapshot file")) }()
	buffer := make([]byte, 64*1024)
	for {
		if err = ctx.Err(); err != nil {
			return errors.Wrap(err, "copy snapshot canceled")
		}
		n, readErr := input.Read(buffer)
		if n > 0 {
			if _, err = output.Write(buffer[:n]); err != nil {
				return errors.Wrap(err, "write snapshot file")
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return errors.Wrap(readErr, "read snapshot file")
		}
	}
}

// syncTree makes every staged file and directory durable before publication.
func syncTree(ctx context.Context, root string) (result error) {
	scoped, err := os.OpenRoot(root)
	if err != nil {
		return errors.Wrap(err, "open scoped snapshot")
	}
	defer func() { result = errors.Join(result, errors.Wrap(scoped.Close(), "close scoped snapshot")) }()
	directories := []string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.Wrap(walkErr, "walk snapshot for sync")
		}
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "sync snapshot canceled")
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("snapshot contains a non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return errors.Wrap(err, "resolve scoped snapshot path")
		}
		file, err := scoped.Open(relative)
		if err != nil {
			return errors.Wrap(err, "open snapshot for sync")
		}
		return errors.Join(
			errors.Wrap(file.Sync(), "sync snapshot file"), errors.Wrap(file.Close(), "close synced snapshot"),
		)
	})
	if err != nil {
		return errors.Wrap(err, "sync snapshot")
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err = syncDirectory(directories[i]); err != nil {
			return err
		}
	}
	return nil
}

// syncDirectory persists the directory entries at path.
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "open directory for sync")
	}
	return errors.Join(
		errors.Wrap(directory.Sync(), "sync directory"), errors.Wrap(directory.Close(), "close synced directory"),
	)
}
