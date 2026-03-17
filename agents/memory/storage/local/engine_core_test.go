package local

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestNewEngineAndClose validates NewEngine input checks and Close behavior.
func TestNewEngineAndClose(t *testing.T) {
	_, err := NewEngine(Config{})
	require.Error(t, err)

	rootDir := t.TempDir()
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	require.NoError(t, engine.Close())

	var nilEngine *Engine
	require.NoError(t, nilEngine.Close())
}

// TestEngineCRUDAndValidation covers Write/Read/Stat/Delete lifecycle and validation errors.
func TestEngineCRUDAndValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	alphaPath := "/docs/alpha.txt"
	require.NoError(t, engine.Write(ctx, project, alphaPath, "hello", memorystorage.WriteModeTruncate, 0))
	require.NoError(t, engine.Write(ctx, project, alphaPath, " world", memorystorage.WriteModeAppend, 0))
	require.NoError(t, engine.Write(ctx, project, alphaPath, "Go", memorystorage.WriteModeOverwrite, 6))

	body, err := engine.Read(ctx, project, alphaPath, 0, -1)
	require.NoError(t, err)
	require.Equal(t, "hello Gorld", body)

	body, err = engine.Read(ctx, project, alphaPath, 6, 5)
	require.NoError(t, err)
	require.Equal(t, "Gorld", body)

	_, err = engine.Read(ctx, project, alphaPath, 999, -1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "offset")

	missingBody, err := engine.Read(ctx, project, "/docs/not-found.txt", 0, -1)
	require.NoError(t, err)
	require.Empty(t, missingBody)

	fileInfo, err := engine.Stat(ctx, project, alphaPath)
	require.NoError(t, err)
	require.True(t, fileInfo.Exists)
	require.Equal(t, memorystorage.FileTypeFile, fileInfo.Type)
	require.NotEmpty(t, fileInfo.UpdatedAt)

	dirInfo, err := engine.Stat(ctx, project, "/docs")
	require.NoError(t, err)
	require.True(t, dirInfo.Exists)
	require.Equal(t, memorystorage.FileTypeDirectory, dirInfo.Type)

	notExistInfo, err := engine.Stat(ctx, project, "/docs/missing")
	require.NoError(t, err)
	require.False(t, notExistInfo.Exists)

	require.NoError(t, engine.Delete(ctx, project, alphaPath, false))
	deletedInfo, err := engine.Stat(ctx, project, alphaPath)
	require.NoError(t, err)
	require.False(t, deletedInfo.Exists)

	require.NoError(t, engine.Write(ctx, project, "/docs/nested/a.txt", "x", memorystorage.WriteModeTruncate, 0))
	require.NoError(t, engine.Delete(ctx, project, "/docs", true))

	docsInfo, err := engine.Stat(ctx, project, "/docs")
	require.NoError(t, err)
	require.False(t, docsInfo.Exists)

	require.NoError(t, engine.Delete(ctx, project, "/docs/no-file", false))

	err = engine.Write(ctx, project, "/docs/offset.txt", "x", memorystorage.WriteModeOverwrite, 9)
	require.Error(t, err)
	require.Contains(t, err.Error(), "offset")

	err = engine.Write(ctx, project, "/docs/invalid-mode.txt", "x", memorystorage.WriteMode("invalid"), 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported write mode")

	_, err = engine.Read(ctx, project, "/docs/offset.txt", -1, -1)
	require.Error(t, err)
	err = engine.Write(ctx, project, "/docs/offset.txt", "x", memorystorage.WriteModeTruncate, -1)
	require.Error(t, err)

	err = engine.Write(ctx, project, "docs/invalid.txt", "x", memorystorage.WriteModeTruncate, 0)
	require.Error(t, err)
	_, err = engine.Stat(ctx, project, "docs/invalid")
	require.Error(t, err)
	err = engine.Delete(ctx, project, "docs/invalid", false)
	require.Error(t, err)

	err = engine.Write(ctx, "bad project/space", "/docs/a.txt", "x", memorystorage.WriteModeTruncate, 0)
	require.Error(t, err)
}

// TestEngineListAndSearchControlPaths covers list/search defaults, limit behavior, and root handling.
func TestEngineListAndSearchControlPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	require.NoError(t, engine.Write(ctx, project, "/items/a.txt", "one token", memorystorage.WriteModeTruncate, 0))
	require.NoError(t, engine.Write(ctx, project, "/items/b.txt", "two token", memorystorage.WriteModeTruncate, 0))
	require.NoError(t, engine.Write(ctx, project, "/items/deep/c.txt", "three token", memorystorage.WriteModeTruncate, 0))

	entries, hasMore, err := engine.List(ctx, project, "/items", 1, 1)
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Len(t, entries, 1)

	entries, hasMore, err = engine.List(ctx, project, "/items", -1, -1)
	require.NoError(t, err)
	require.False(t, hasMore)
	require.NotEmpty(t, entries)

	entries, hasMore, err = engine.List(ctx, project, "/not-found", 8, 100)
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Empty(t, entries)

	chunks, err := engine.Search(ctx, project, "token", "/items", -1)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	chunks, err = engine.Search(ctx, project, "   ", "/items", 5)
	require.Error(t, err)
	require.Empty(t, chunks)

	chunks, err = engine.Search(ctx, project, "token", "/not-found", 5)
	require.NoError(t, err)
	require.Empty(t, chunks)
}

// TestEngineContextCancellation verifies all operations return context errors when context is canceled.
func TestEngineContextCancellation(t *testing.T) {
	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = engine.Read(ctx, project, "/a.txt", 0, -1)
	require.Error(t, err)

	err = engine.Write(ctx, project, "/a.txt", "x", memorystorage.WriteModeTruncate, 0)
	require.Error(t, err)

	_, err = engine.Stat(ctx, project, "/a.txt")
	require.Error(t, err)

	_, _, err = engine.List(ctx, project, "/", 1, 1)
	require.Error(t, err)

	_, err = engine.Search(ctx, project, "x", "/", 1)
	require.Error(t, err)

	err = engine.Delete(ctx, project, "/a.txt", false)
	require.Error(t, err)
}

// TestHelpers verifies standalone helper behavior in normal and invalid cases.
func TestHelpers(t *testing.T) {
	require.NoError(t, validateProject("ok-Project_123"))
	require.Error(t, validateProject("bad project"))

	rel, abs, err := normalizePath("/a/b.txt", false)
	require.NoError(t, err)
	require.Equal(t, "a/b.txt", rel)
	require.Equal(t, "/a/b.txt", abs)

	_, _, err = normalizePath("", false)
	require.Error(t, err)
	_, _, err = normalizePath("/", false)
	require.Error(t, err)
	_, _, err = normalizePath("a/b", false)
	require.Error(t, err)
	_, _, err = normalizePath("/a//b", false)
	require.Error(t, err)
	_, _, err = normalizePath("/a/../b", false)
	require.Error(t, err)
	_, _, err = normalizePath("/a b", false)
	require.Error(t, err)
	_, _, err = normalizePath("/a/", false)
	require.Error(t, err)
	_, _, err = normalizePath("/a/./b", false)
	require.Error(t, err)
	_, _, err = normalizePath("/a/..", false)
	require.Error(t, err)
	_, _, err = normalizePath("/a/../b", false)
	require.Error(t, err)

	rel, abs, err = normalizePath("/", true)
	require.NoError(t, err)
	require.Equal(t, ".", rel)
	require.Equal(t, "/", abs)

	require.Equal(t, 0, calculateRelativeDepth(".", "."))
	require.Equal(t, 1, calculateRelativeDepth(".", "a"))
	require.Equal(t, 2, calculateRelativeDepth(".", "a/b"))
	require.Equal(t, 0, calculateRelativeDepth("a", "a"))
	require.Equal(t, 1, calculateRelativeDepth("a", "a/b"))

	require.Equal(t, "/", toStoragePath("."))
	require.Equal(t, "/a/b", toStoragePath("./a/b"))

	require.Equal(t, memorystorage.FileTypeDirectory, fileTypeFromMode(fs.ModeDir))
	require.Equal(t, memorystorage.FileTypeFile, fileTypeFromMode(0))
	require.Equal(t, memorystorage.FileTypeUnknown, fileTypeFromMode(fs.ModeCharDevice))

	require.Equal(t, 2, minInt(2, 5))
	require.Equal(t, 1, minInt(7, 1))

	require.NoError(t, wrapTraversalError("listing", "/x", os.ErrNotExist))
	err = wrapTraversalError("listing", "/x", fs.ErrPermission)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
	err = wrapTraversalError("listing", "/x", fs.ErrInvalid)
	require.Error(t, err)
	require.Contains(t, err.Error(), "walk path")

	require.NoError(t, wrapEntryInfoError("/x", os.ErrNotExist))
	err = wrapEntryInfoError("/x", fs.ErrPermission)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
	err = wrapEntryInfoError("/x", fs.ErrInvalid)
	require.Error(t, err)
	require.Contains(t, err.Error(), "load entry info")

	require.NoError(t, wrapSearchReadError("/x", os.ErrNotExist))
	err = wrapSearchReadError("/x", fs.ErrPermission)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
	err = wrapSearchReadError("/x", fs.ErrInvalid)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read file")
}

// TestWriteHelpers validates writeAppend and writeOverwrite helper semantics directly.
func TestWriteHelpers(t *testing.T) {
	rootDir := t.TempDir()
	root, err := os.OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()

	require.NoError(t, os.MkdirAll(filepath.Join(rootDir, "a"), 0o755))

	require.NoError(t, writeAppend(root, "a/file.txt", "hello", 0o644))
	require.NoError(t, writeAppend(root, "a/file.txt", " world", 0o644))
	body, err := root.ReadFile("a/file.txt")
	require.NoError(t, err)
	require.Equal(t, "hello world", string(body))

	require.NoError(t, writeOverwrite(root, "a/file.txt", "Go", 6, 0o644))
	body, err = root.ReadFile("a/file.txt")
	require.NoError(t, err)
	require.Equal(t, "hello Gorld", string(body))

	err = writeOverwrite(root, "a/file.txt", "x", 999, 0o644)
	require.Error(t, err)
	require.Contains(t, err.Error(), "offset")
}

// TestEngineAdditionalErrorBranches drives less common branches that are hard to hit in integration-like flows.
func TestEngineAdditionalErrorBranches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	require.NoError(t, engine.Write(ctx, project, "/slice.txt", "abcdef", memorystorage.WriteModeTruncate, 0))
	body, err := engine.Read(ctx, project, "/slice.txt", 1, 3)
	require.NoError(t, err)
	require.Equal(t, "bcd", body)

	_, err = engine.Read(ctx, "bad project", "/slice.txt", 0, -1)
	require.Error(t, err)
	_, err = engine.Read(ctx, "not-existing-project", "/slice.txt", 0, -1)
	require.NoError(t, err)

	require.NoError(t, engine.Write(ctx, project, "/dir/as-file.txt", "x", memorystorage.WriteModeTruncate, 0))
	_, err = engine.Read(ctx, project, "/dir", 0, -1)
	require.Error(t, err)

	require.NoError(t, engine.Write(ctx, project, "/parent-file", "x", memorystorage.WriteModeTruncate, 0))
	err = engine.Write(ctx, project, "/parent-file/child.txt", "x", memorystorage.WriteModeTruncate, 0)
	require.Error(t, err)

	require.NoError(t, engine.Write(ctx, project, "/a-dir/file.txt", "x", memorystorage.WriteModeTruncate, 0))
	err = engine.Write(ctx, project, "/a-dir", "x", memorystorage.WriteModeTruncate, 0)
	require.Error(t, err)
	err = engine.Write(ctx, project, "/a-dir", "x", memorystorage.WriteModeAppend, 0)
	require.Error(t, err)
	err = engine.Write(ctx, project, "/a-dir", "x", memorystorage.WriteModeOverwrite, 0)
	require.Error(t, err)

	_, err = engine.Stat(ctx, "bad project", "/slice.txt")
	require.Error(t, err)
	info, err := engine.Stat(ctx, "missing-project", "/slice.txt")
	require.NoError(t, err)
	require.False(t, info.Exists)
	_, err = engine.Stat(ctx, project, "/parent-file/child")
	require.Error(t, err)

	_, _, err = engine.List(ctx, project, "not-absolute", 1, 1)
	require.Error(t, err)
	_, _, err = engine.List(ctx, "bad project", "/", 1, 1)
	require.Error(t, err)
	entries, hasMore, err := engine.List(ctx, "missing-project", "/", 1, 1)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.False(t, hasMore)
	_, _, err = engine.List(ctx, project, "/parent-file/child", maxListDepth+10, maxListLimit+10)
	require.Error(t, err)

	_, err = engine.Search(ctx, project, "x", "not-absolute", 1)
	require.Error(t, err)
	_, err = engine.Search(ctx, "bad project", "x", "/", 1)
	require.Error(t, err)
	chunks, err := engine.Search(ctx, "missing-project", "x", "/", 1)
	require.NoError(t, err)
	require.Empty(t, chunks)
	_, err = engine.Search(ctx, project, "x", "/parent-file/child", maxSearchLimit+10)
	require.Error(t, err)

	err = engine.Delete(ctx, "bad project", "/slice.txt", false)
	require.Error(t, err)
	err = engine.Delete(ctx, "missing-project", "/slice.txt", false)
	require.NoError(t, err)

	err = engine.Delete(ctx, project, "/a-dir", false)
	require.Error(t, err)
	err = engine.Delete(ctx, project, "/parent-file/child", true)
	require.Error(t, err)
}

// TestNewEngineCloseErrorAndPathEdgeCases covers path-related constructor failures and close-on-closed-root behavior.
func TestNewEngineCloseErrorAndPathEdgeCases(t *testing.T) {
	rootDir := t.TempDir()
	fileAsRoot := filepath.Join(rootDir, "not-a-dir")
	require.NoError(t, os.WriteFile(fileAsRoot, []byte("x"), 0o644))

	_, err := NewEngine(Config{RootDir: fileAsRoot})
	require.Error(t, err)

	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	require.NoError(t, engine.Close())
	require.NoError(t, engine.Close())
}

// TestOpenProjectRootDirectBranches covers direct openProjectRoot branches that are awkward through public methods.
func TestOpenProjectRootDirectBranches(t *testing.T) {
	rootDir := t.TempDir()
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	rootHandle, err := os.OpenRoot(rootDir)
	require.NoError(t, err)
	require.NoError(t, rootHandle.WriteFile("project-as-file", []byte("x"), 0o644))
	require.NoError(t, rootHandle.Close())

	_, err = engine.openProjectRoot("bad project", false)
	require.Error(t, err)

	missingRoot, err := engine.openProjectRoot("missing-project", false)
	require.ErrorIs(t, err, errProjectRootNotFound)
	require.Nil(t, missingRoot)

	_, err = engine.openProjectRoot("project-as-file", true)
	require.Error(t, err)

	_, err = engine.openProjectRoot("project-as-file", false)
	require.Error(t, err)
}
