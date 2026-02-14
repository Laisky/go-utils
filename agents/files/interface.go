package files

import "context"

// WriteMode defines how file content is written.
//
// APPEND writes to EOF, OVERWRITE writes from offset without truncating tail,
// and TRUNCATE clears file before writing.
type WriteMode string

const (
	// WriteModeAppend appends content to the target file.
	WriteModeAppend WriteMode = "APPEND"
	// WriteModeOverwrite overwrites content from a byte offset.
	WriteModeOverwrite WriteMode = "OVERWRITE"
	// WriteModeTruncate truncates file and writes from zero offset.
	WriteModeTruncate WriteMode = "TRUNCATE"
)

// FileType represents the kind of path returned by storage stat/list.
type FileType string

const (
	// FileTypeFile represents a regular file.
	FileTypeFile FileType = "FILE"
	// FileTypeDirectory represents a directory.
	FileTypeDirectory FileType = "DIRECTORY"
	// FileTypeUnknown represents an unknown path type.
	FileTypeUnknown FileType = "UNKNOWN"
)

// FileChunk is a chunk of search hit content.
type FileChunk struct {
	FilePath   string
	StartBytes int64
	EndBytes   int64
	Content    string
	Score      float64
}

// FileInfo describes metadata of a storage path.
type FileInfo struct {
	Path      string
	Exists    bool
	Type      FileType
	SizeBytes int64
	UpdatedAt string
}

// Storage defines a pluggable file storage abstraction for agent memory.
type Storage interface {
	Read(ctx context.Context, project, path string, offset, length int64) (string, error)
	Write(ctx context.Context, project, path, content string, mode WriteMode, offset int64) error
	Stat(ctx context.Context, project, path string) (FileInfo, error)
	List(ctx context.Context, project, path string, depth, limit int) (entries []FileInfo, hasMore bool, err error)
	Search(ctx context.Context, project, query, pathPrefix string, limit int) ([]FileChunk, error)
	Delete(ctx context.Context, project, path string, recursive bool) error
}
