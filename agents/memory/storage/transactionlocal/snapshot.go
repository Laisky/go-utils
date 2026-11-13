//go:build linux

package transactionlocal

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

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
	type directoryTimestamp struct {
		path string
		info fs.FileInfo
	}
	directories := []directoryTimestamp{}
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
		info, err := entry.Info()
		if err != nil {
			return errors.Wrap(err, "stat generation entry")
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			if relative != "." {
				// Staging must permit descendant creation even when the source
				// directory is readable/traversable but intentionally read-only.
				// Exact source permissions are restored after all children exist.
				stagingMode := info.Mode().Perm() | 0o700
				if err = os.Mkdir(destination, stagingMode); err != nil {
					return errors.Wrap(err, "copy generation directory")
				}
				if err = os.Chmod(destination, stagingMode); err != nil {
					return errors.Wrap(err, "prepare staging directory permissions")
				}
			}
			directories = append(directories, directoryTimestamp{path: destination, info: info})
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("generation contains a non-regular file")
		}
		if err = copyFile(ctx, path, destination, info.Mode().Perm()); err != nil {
			return err
		}
		return preserveTimestamp(destination, info)
	})
	if err != nil {
		return errors.Wrap(err, "copy generation")
	}
	// Restore directories after their children are created. Later caller mutations
	// update only the directories they actually change, as in the original tree.
	for i := len(directories) - 1; i >= 0; i-- {
		if err = os.Chmod(directories[i].path, directories[i].info.Mode().Perm()); err != nil {
			return errors.Wrap(err, "preserve snapshot directory permissions")
		}
		if err = preserveTimestamp(directories[i].path, directories[i].info); err != nil {
			return err
		}
	}
	return nil
}

// preserveTimestamp restores the original entry's access and modification times.
func preserveTimestamp(path string, info fs.FileInfo) error {
	access := info.ModTime()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		access = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
	}
	return errors.Wrap(os.Chtimes(path, access, info.ModTime()), "preserve snapshot timestamp")
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
	if err = output.Chmod(mode); err != nil {
		return errors.Wrap(err, "preserve snapshot file permissions")
	}
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
