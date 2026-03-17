package local

import (
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

var projectRegex = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// writeAppend opens or creates a file and appends content to EOF.
//
// Parameters:
//   - root: Project-specific os.Root handle.
//   - relPath: Relative file path within the project root.
//   - content: Content to append.
//   - perm: File permission used when creating a new file.
//
// Returns:
//   - error: Non-nil when file open or write fails.
func writeAppend(root *os.Root, relPath, content string, perm os.FileMode) error {
	file, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, perm)
	if err != nil {
		return errors.Wrap(err, "open append file")
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err = io.WriteString(file, content); err != nil {
		return errors.Wrap(err, "append file content")
	}

	return nil
}

// writeOverwrite opens or creates a file and overwrites from a fixed offset.
//
// Parameters:
//   - root: Project-specific os.Root handle.
//   - relPath: Relative file path within the project root.
//   - content: Content to overwrite from the offset.
//   - offset: Zero-based byte offset used as the write start position.
//   - perm: File permission used when creating a new file.
//
// Returns:
//   - error: Non-nil when validation, seek, or write fails.
func writeOverwrite(root *os.Root, relPath, content string, offset int64, perm os.FileMode) error {
	file, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY, perm)
	if err != nil {
		return errors.Wrap(err, "open overwrite file")
	}
	defer func() {
		_ = file.Close()
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return errors.Wrap(err, "stat overwrite file")
	}
	if offset > fileInfo.Size() {
		return errors.Errorf("offset out of range")
	}

	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return errors.Wrap(err, "seek overwrite file")
	}
	if _, err = io.WriteString(file, content); err != nil {
		return errors.Wrap(err, "write overwrite content")
	}

	return nil
}

// validateProject validates project identifier format.
//
// Parameters:
//   - project: Project namespace string.
//
// Returns:
//   - error: Non-nil when project is empty or has invalid characters.
func validateProject(project string) error {
	if !projectRegex.MatchString(project) {
		return errors.Errorf("invalid project `%s`", project)
	}

	return nil
}

// normalizePath validates one storage path and returns relative and normalized absolute forms.
//
// Parameters:
//   - storagePath: Absolute storage path to validate.
//   - allowRoot: Whether empty path or slash path should be treated as root.
//
// Returns:
//   - string: Relative path used with os.Root APIs.
//   - string: Normalized absolute storage path.
//   - error: Non-nil when path format is invalid.
func normalizePath(storagePath string, allowRoot bool) (string, string, error) {
	trimmed := strings.TrimSpace(storagePath)
	if trimmed == "" || trimmed == "/" {
		if allowRoot {
			return ".", "/", nil
		}
		return "", "", errors.Errorf("path cannot be empty")
	}

	if !strings.HasPrefix(trimmed, "/") {
		return "", "", errors.Errorf("path must start with `/`")
	}
	if strings.HasSuffix(trimmed, "/") {
		return "", "", errors.Errorf("path cannot end with `/`")
	}
	if strings.Contains(trimmed, "//") {
		return "", "", errors.Errorf("path cannot contain `//`")
	}
	if strings.Contains(trimmed, "/./") || strings.HasPrefix(trimmed, "/./") || strings.HasSuffix(trimmed, "/.") {
		return "", "", errors.Errorf("path cannot contain `.` segment")
	}
	if strings.Contains(trimmed, "/../") || strings.HasPrefix(trimmed, "/../") || strings.HasSuffix(trimmed, "/..") {
		return "", "", errors.Errorf("path cannot contain `..` segment")
	}
	if strings.ContainsAny(trimmed, " \t\n\r") {
		return "", "", errors.Errorf("path cannot contain spaces or control chars")
	}
	if len(trimmed) > 512 {
		return "", "", errors.Errorf("path too long")
	}

	normalized := path.Clean(trimmed)
	if normalized != trimmed {
		return "", "", errors.Errorf("path is not canonical")
	}

	relPath := strings.TrimPrefix(normalized, "/")
	if relPath == "" {
		if allowRoot {
			return ".", "/", nil
		}
		return "", "", errors.Errorf("path cannot be root")
	}

	return relPath, normalized, nil
}

// calculateRelativeDepth calculates current path depth relative to a traversal root.
//
// Parameters:
//   - startRelPath: Walk start path in relative form.
//   - currentPath: Current path visited during traversal.
//
// Returns:
//   - int: Relative depth where 0 means the traversal root itself.
func calculateRelativeDepth(startRelPath, currentPath string) int {
	if currentPath == startRelPath {
		return 0
	}

	if startRelPath == "." {
		trimmed := strings.TrimPrefix(currentPath, "./")
		if trimmed == "" || trimmed == "." {
			return 0
		}
		return strings.Count(trimmed, "/") + 1
	}

	trimmed := strings.TrimPrefix(currentPath, startRelPath)
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return 0
	}

	return strings.Count(trimmed, "/") + 1
}

// toStoragePath converts a relative path returned by fs.WalkDir into absolute storage path.
//
// Parameters:
//   - relPath: Relative filesystem path returned by traversal.
//
// Returns:
//   - string: Normalized absolute storage path beginning with '/'.
func toStoragePath(relPath string) string {
	clean := path.Clean(strings.TrimPrefix(relPath, "./"))
	if clean == "." {
		return "/"
	}

	return "/" + clean
}

// fileTypeFromMode maps fs.FileMode to the normalized storage file type.
//
// Parameters:
//   - mode: File mode of one filesystem entry.
//
// Returns:
//   - memorystorage.FileType: The mapped file type value.
func fileTypeFromMode(mode fs.FileMode) memorystorage.FileType {
	if mode.IsDir() {
		return memorystorage.FileTypeDirectory
	}
	if mode.IsRegular() {
		return memorystorage.FileTypeFile
	}

	return memorystorage.FileTypeUnknown
}

// isSymlinkEntry reports whether one directory entry is a symbolic link.
//
// Parameters:
//   - entry: Directory entry discovered during traversal.
//
// Returns:
//   - bool: True when entry type indicates a symbolic link.
func isSymlinkEntry(entry fs.DirEntry) bool {
	return entry.Type()&fs.ModeSymlink != 0
}

// wrapTraversalError normalizes traversal-time errors for list/search operations.
//
// Parameters:
//   - operation: Human-readable operation name, such as listing or searching.
//   - currentPath: The current traversal path.
//   - err: The original traversal error.
//
// Returns:
//   - error: Nil for not-exist races, otherwise a wrapped actionable error.
func wrapTraversalError(operation, currentPath string, err error) error {
	if os.IsNotExist(err) {
		return nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return errors.Wrapf(err, "permission denied while %s `%s`", operation, currentPath)
	}

	return errors.Wrapf(err, "walk path `%s`", currentPath)
}

// wrapEntryInfoError normalizes errors raised while loading one dir-entry metadata.
//
// Parameters:
//   - currentPath: The current traversal path.
//   - err: The original metadata error.
//
// Returns:
//   - error: Nil for not-exist races, otherwise a wrapped actionable error.
func wrapEntryInfoError(currentPath string, err error) error {
	if os.IsNotExist(err) {
		return nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return errors.Wrapf(err, "permission denied while loading info `%s`", currentPath)
	}

	return errors.Wrapf(err, "load entry info `%s`", currentPath)
}

// wrapSearchReadError normalizes errors raised while reading one file body in search.
//
// Parameters:
//   - currentPath: The file path being read.
//   - err: The original read error.
//
// Returns:
//   - error: Nil for not-exist races, otherwise a wrapped actionable error.
func wrapSearchReadError(currentPath string, err error) error {
	if os.IsNotExist(err) {
		return nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return errors.Wrapf(err, "permission denied while reading `%s` for search", currentPath)
	}

	return errors.Wrapf(err, "read file `%s` for search", currentPath)
}

// minInt returns the smaller integer value.
//
// Parameters:
//   - a: First integer.
//   - b: Second integer.
//
// Returns:
//   - int: The smaller value.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
