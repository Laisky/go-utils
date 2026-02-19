package storage

import (
	"context"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// WriteMode defines how file content is written by storage engines.
type WriteMode = files.WriteMode

const (
	// WriteModeAppend appends content to the target file.
	WriteModeAppend WriteMode = files.WriteModeAppend
	// WriteModeOverwrite overwrites content from an offset while keeping the remaining tail.
	WriteModeOverwrite WriteMode = files.WriteModeOverwrite
	// WriteModeTruncate truncates the target file before writing.
	WriteModeTruncate WriteMode = files.WriteModeTruncate
)

// FileType is the normalized storage path type.
type FileType = files.FileType

const (
	// FileTypeFile represents a regular file path.
	FileTypeFile FileType = files.FileTypeFile
	// FileTypeDirectory represents a directory path.
	FileTypeDirectory FileType = files.FileTypeDirectory
	// FileTypeUnknown represents an unknown path type.
	FileTypeUnknown FileType = files.FileTypeUnknown
)

// FileChunk is one search result chunk from a storage engine.
type FileChunk = files.FileChunk

// FileInfo describes metadata of a storage path.
type FileInfo = files.FileInfo

// Engine defines the standard memory storage engine contract.
type Engine interface {
	Read(ctx context.Context, project, path string, offset, length int64) (string, error)
	Write(ctx context.Context, project, path, content string, mode WriteMode, offset int64) error
	Stat(ctx context.Context, project, path string) (FileInfo, error)
	List(ctx context.Context, project, path string, depth, limit int) (entries []FileInfo, hasMore bool, err error)
	Search(ctx context.Context, project, query, pathPrefix string, limit int) ([]FileChunk, error)
	Delete(ctx context.Context, project, path string, recursive bool) error
}

// FromFilesStorage adapts an agents/files storage into the memory storage engine interface.
//
// Parameters:
//   - storage: The source files storage implementation.
//
// Returns:
//   - Engine: The same implementation exposed through the memory storage engine interface.
func FromFilesStorage(storage files.Storage) Engine {
	return storage
}
