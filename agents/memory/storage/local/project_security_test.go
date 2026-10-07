package local

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestSecurity51DotProjectCannotReachSibling exercises every public storage operation against a disposable sibling project.
func TestSecurity51DotProjectCannotReachSibling(t *testing.T) {
	operations := map[string]func(context.Context, *Engine) error{
		"read": func(ctx context.Context, e *Engine) error {
			_, err := e.Read(ctx, ".", "/victim/note.txt", 0, -1)
			return err
		},
		"stat": func(ctx context.Context, e *Engine) error {
			_, err := e.Stat(ctx, ".", "/victim/note.txt")
			return err
		},
		"list": func(ctx context.Context, e *Engine) error {
			_, _, err := e.List(ctx, ".", "/", 2, 10)
			return err
		},
		"search": func(ctx context.Context, e *Engine) error {
			_, err := e.Search(ctx, ".", "SENTINEL_ONLY", "/", 5)
			return err
		},
		"write": func(ctx context.Context, e *Engine) error {
			return e.Write(ctx, ".", "/victim/note.txt", "CHANGED", memorystorage.WriteModeTruncate, 0)
		},
		"delete": func(ctx context.Context, e *Engine) error {
			return e.Delete(ctx, ".", "/victim/note.txt", false)
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			engine, err := NewEngine(Config{RootDir: t.TempDir()})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, engine.Close()) })
			require.NoError(t, engine.Write(ctx, "victim", "/note.txt", "SENTINEL_ONLY", memorystorage.WriteModeTruncate, 0))
			require.Error(t, operation(ctx, engine))
			body, err := engine.Read(ctx, "victim", "/note.txt", 0, -1)
			require.NoError(t, err)
			require.Equal(t, "SENTINEL_ONLY", body)
		})
	}
}

// TestSecurity51CanonicalProjectNames checks reserved components before project handles can be opened or created.
func TestSecurity51CanonicalProjectNames(t *testing.T) {
	for _, project := range []string{"", ".", "..", "...", "name.", "/", "a/b", `a\b`, "a:b", " a", "a ", "中文", strings.Repeat("a", 129)} {
		t.Run("invalid/"+project, func(t *testing.T) {
			require.Error(t, validateProject(project))
		})
	}
	for _, project := range []string{"a", "project-1", "project_name", "a.b", "a..b", ".hidden", strings.Repeat("a", 128)} {
		t.Run("valid/"+project, func(t *testing.T) {
			require.NoError(t, validateProject(project))
			engine, err := NewEngine(Config{RootDir: t.TempDir()})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, engine.Close()) })
			require.NoError(t, engine.Write(context.Background(), project, "/note", "ok", memorystorage.WriteModeTruncate, 0))
			body, err := engine.Read(context.Background(), project, "/note", 0, -1)
			require.NoError(t, err)
			require.Equal(t, "ok", body)
		})
	}
}
