package local

import (
	"context"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

var (
	errWalkStop            = errors.New("walk stopped because result limit was reached")
	errProjectRootNotFound = errors.New("project root not found")
)

const (
	// defaultListDepth is the fallback traversal depth when callers pass non-positive depth.
	defaultListDepth = 8
	// defaultListLimit is the fallback maximum entry count when callers pass non-positive limit.
	defaultListLimit = 1000
	// defaultSearchLimit is the fallback maximum chunk count when callers pass non-positive limit.
	defaultSearchLimit = 5
	// maxListDepth is the hard upper bound for list traversal depth.
	maxListDepth = 32
	// maxListLimit is the hard upper bound for list result count.
	maxListLimit = 5000
	// maxSearchLimit is the hard upper bound for search result count.
	maxSearchLimit = 50
	// maxSearchFileBytes is the maximum file size allowed for content-based search scanning.
	maxSearchFileBytes = 4 * 1024 * 1024
)

// Config controls Local storage engine initialization.
type Config struct {
	RootDir  string
	FilePerm os.FileMode
	DirPerm  os.FileMode
}

// Engine is a local filesystem implementation of the memory storage interface.
type Engine struct {
	root     *os.Root
	filePerm os.FileMode
	dirPerm  os.FileMode
}

// NewEngine creates a local storage engine rooted at conf.RootDir.
//
// Parameters:
//   - conf: Local engine configuration including root path and file permissions.
//
// Returns:
//   - *Engine: The initialized local storage engine.
//   - error: Non-nil when the root path is invalid or cannot be opened.
func NewEngine(conf Config) (*Engine, error) {
	if strings.TrimSpace(conf.RootDir) == "" {
		return nil, errors.Errorf("root_dir is required")
	}

	dirPerm := conf.DirPerm
	if dirPerm == 0 {
		// Security: default to owner-only (0o700) so the storage tree is not
		// group/world-readable or traversable. Memory storage can hold sensitive
		// conversation history and extracted facts; on shared hosts a permissive
		// 0o755 default would expose it to other local users. Callers may still
		// override via Config for environments that require wider access.
		dirPerm = 0o700
	}

	// Create the root with the (tightened) directory permission rather than a
	// hard-coded 0o755, so the root itself is not world-traversable by default.
	if err := os.MkdirAll(conf.RootDir, dirPerm); err != nil {
		return nil, errors.Wrap(err, "mkdir local storage root")
	}

	root, err := os.OpenRoot(conf.RootDir)
	if err != nil {
		return nil, errors.Wrap(err, "open local storage root")
	}

	filePerm := conf.FilePerm
	if filePerm == 0 {
		// Security: default to owner-only read/write (0o600) so stored files are
		// not readable by group/other on shared systems.
		filePerm = 0o600
	}

	return &Engine{
		root:     root,
		filePerm: filePerm,
		dirPerm:  dirPerm,
	}, nil
}

// Close releases the underlying os.Root handle.
//
// Parameters:
//   - none.
//
// Returns:
//   - error: Non-nil when closing the root handle fails.
func (engine *Engine) Close() error {
	if engine == nil || engine.root == nil {
		return nil
	}

	if err := engine.root.Close(); err != nil {
		return errors.Wrap(err, "close local root")
	}

	return nil
}

// Read reads content from a file path with optional byte slicing.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path such as /memory/session/context.jsonl.
//   - offset: Start byte offset in the target file.
//   - length: Number of bytes to read; -1 means read to EOF.
//
// Returns:
//   - string: The requested file content slice, or empty string when file does not exist.
//   - error: Non-nil when validation fails or filesystem access fails.
func (engine *Engine) Read(ctx context.Context, project, storagePath string, offset, length int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.Wrap(err, "context done")
	}

	relPath, _, err := normalizePath(storagePath, false)
	if err != nil {
		return "", errors.Wrap(err, "normalize path")
	}

	if offset < 0 {
		return "", errors.Errorf("offset must be >= 0")
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return "", nil
		}
		return "", errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return "", nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	body, err := projectRoot.ReadFile(relPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(err, "read file")
	}

	if offset > int64(len(body)) {
		return "", errors.Errorf("offset out of range")
	}

	body = body[offset:]
	if length >= 0 && length < int64(len(body)) {
		body = body[:length]
	}

	return string(body), nil
}

// Write writes content into a file using append, overwrite, or truncate mode.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path such as /memory/session/context.jsonl.
//   - content: UTF-8 text content to write.
//   - mode: Write mode controlling append/overwrite/truncate behavior.
//   - offset: Start offset for overwrite mode.
//
// Returns:
//   - error: Non-nil when validation fails or write operations fail.
func (engine *Engine) Write(
	ctx context.Context,
	project, storagePath, content string,
	mode memorystorage.WriteMode,
	offset int64,
) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done")
	}

	relPath, _, err := normalizePath(storagePath, false)
	if err != nil {
		return errors.Wrap(err, "normalize path")
	}

	if offset < 0 {
		return errors.Errorf("offset must be >= 0")
	}

	projectRoot, err := engine.openProjectRoot(project, true)
	if err != nil {
		return errors.Wrap(err, "open project root")
	}
	if errors.Is(err, errProjectRootNotFound) {
		return errors.Errorf("project root is nil")
	}
	if projectRoot == nil {
		return errors.Errorf("project root is nil")
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	parentDir := path.Dir(relPath)
	if parentDir != "." {
		if err = projectRoot.MkdirAll(parentDir, engine.dirPerm); err != nil {
			return errors.Wrap(err, "mkdir parent directories")
		}
	}

	switch mode {
	case memorystorage.WriteModeAppend:
		if err = writeAppend(projectRoot, relPath, content, engine.filePerm); err != nil {
			return errors.Wrap(err, "append content")
		}
	case memorystorage.WriteModeOverwrite:
		if err = writeOverwrite(projectRoot, relPath, content, offset, engine.filePerm); err != nil {
			return errors.Wrap(err, "overwrite content")
		}
	case memorystorage.WriteModeTruncate:
		if err = projectRoot.WriteFile(relPath, []byte(content), engine.filePerm); err != nil {
			return errors.Wrap(err, "truncate and write content")
		}
	default:
		return errors.Errorf("unsupported write mode `%s`", mode)
	}

	return nil
}

// Stat returns metadata for a target path inside one project namespace.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path or root marker (/ or empty when allow-root callers use it).
//
// Returns:
//   - memorystorage.FileInfo: Metadata describing existence, type, size, and update time.
//   - error: Non-nil when validation fails or stat operations fail.
func (engine *Engine) Stat(ctx context.Context, project, storagePath string) (memorystorage.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return memorystorage.FileInfo{}, errors.Wrap(err, "context done")
	}

	relPath, normalizedPath, err := normalizePath(storagePath, true)
	if err != nil {
		return memorystorage.FileInfo{}, errors.Wrap(err, "normalize path")
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return memorystorage.FileInfo{
				Path:   normalizedPath,
				Exists: false,
				Type:   memorystorage.FileTypeUnknown,
			}, nil
		}
		return memorystorage.FileInfo{}, errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return memorystorage.FileInfo{Path: normalizedPath, Exists: false, Type: memorystorage.FileTypeUnknown}, nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	fileInfo, err := projectRoot.Stat(relPath)
	if err != nil {
		if os.IsNotExist(err) {
			return memorystorage.FileInfo{Path: normalizedPath, Exists: false, Type: memorystorage.FileTypeUnknown}, nil
		}
		return memorystorage.FileInfo{}, errors.Wrap(err, "stat path")
	}

	return memorystorage.FileInfo{
		Path:      normalizedPath,
		Exists:    true,
		Type:      fileTypeFromMode(fileInfo.Mode()),
		SizeBytes: fileInfo.Size(),
		UpdatedAt: fileInfo.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// Delete deletes one path or recursively removes a directory subtree.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path to delete.
//   - recursive: Whether directory deletion should remove descendants.
//
// Returns:
//   - error: Non-nil when validation fails or deletion fails.
func (engine *Engine) Delete(ctx context.Context, project, storagePath string, recursive bool) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done")
	}

	relPath, _, err := normalizePath(storagePath, false)
	if err != nil {
		return errors.Wrap(err, "normalize path")
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return nil
		}
		return errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	if recursive {
		if err = projectRoot.RemoveAll(relPath); err != nil && !os.IsNotExist(err) {
			return errors.Wrap(err, "remove all path")
		}
		return nil
	}

	if err = projectRoot.Remove(relPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove path")
	}

	return nil
}

// openProjectRoot opens one project namespace root and optionally creates it first.
//
// Parameters:
//   - project: Project namespace name.
//   - create: Whether missing project directory should be created.
//
// Returns:
//   - *os.Root: Opened project root, or nil when project does not exist and create=false.
//   - error: Non-nil when project validation or root operations fail.
func (engine *Engine) openProjectRoot(project string, create bool) (*os.Root, error) {
	if err := validateProject(project); err != nil {
		return nil, errors.Wrap(err, "validate project")
	}

	if create {
		if err := engine.root.MkdirAll(project, engine.dirPerm); err != nil {
			return nil, errors.Wrap(err, "mkdir project root")
		}
	}

	projectRoot, err := engine.root.OpenRoot(project)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errProjectRootNotFound
		}
		return nil, errors.Wrap(err, "open project root")
	}

	return projectRoot, nil
}
