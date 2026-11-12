//go:build linux

package transactionlocal

import (
	"context"

	"github.com/Laisky/errors/v2"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// transactionView restricts callback operations to one project and lifetime.
// Callbacks use this view synchronously and must not retain it or start goroutines.
type transactionView struct {
	storage.Engine
	project string
	active  bool
	failure error
}

// check rejects operations outside the active callback and project.
func (view *transactionView) check(project string) error {
	if !view.active {
		return errors.New("transaction view is closed")
	}
	if project != view.project {
		return errors.New("transaction view project mismatch")
	}
	return nil
}

// Read applies the local storage operation to the active transaction view.
func (view *transactionView) Read(ctx context.Context, project, path string, offset, length int64) (string, error) {
	if err := view.check(project); err != nil {
		return "", err
	}
	return view.Engine.Read(ctx, project, path, offset, length)
}

// Read performs the local storage operation under the shared filesystem lock.
func (engine *Engine) Read(ctx context.Context, project, path string, offset, length int64) (string, error) {
	var value string
	err := engine.readView(ctx, func(backend storage.Engine) (err error) {
		value, err = backend.Read(ctx, project, path, offset, length)
		return err
	})
	return value, err
}

// Write applies the local storage operation to the active transaction view.
func (view *transactionView) Write(
	ctx context.Context, project, path, content string, mode storage.WriteMode, offset int64,
) error {
	if err := view.check(project); err != nil {
		view.failure = errors.Join(view.failure, err)
		return err
	}
	err := view.Engine.Write(ctx, project, path, content, mode, offset)
	view.failure = errors.Join(view.failure, err)
	return err
}

// Write performs the local storage operation under the shared filesystem lock.
func (engine *Engine) Write(
	ctx context.Context, project, path, content string, mode storage.WriteMode, offset int64,
) error {
	return engine.Transaction(ctx, project, "", func(backend storage.Engine) error {
		return backend.Write(ctx, project, path, content, mode, offset)
	})
}

// Stat applies the local storage operation to the active transaction view.
func (view *transactionView) Stat(ctx context.Context, project, path string) (storage.FileInfo, error) {
	if err := view.check(project); err != nil {
		return storage.FileInfo{}, err
	}
	return view.Engine.Stat(ctx, project, path)
}

// Stat performs the local storage operation under the shared filesystem lock.
func (engine *Engine) Stat(ctx context.Context, project, path string) (storage.FileInfo, error) {
	var value storage.FileInfo
	err := engine.readView(ctx, func(backend storage.Engine) (err error) {
		value, err = backend.Stat(ctx, project, path)
		return err
	})
	return value, err
}

// List applies the local storage operation to the active transaction view.
func (view *transactionView) List(
	ctx context.Context, project, path string, depth, limit int,
) ([]storage.FileInfo, bool, error) {
	if err := view.check(project); err != nil {
		return nil, false, err
	}
	return view.Engine.List(ctx, project, path, depth, limit)
}

// List performs the local storage operation under the shared filesystem lock.
func (engine *Engine) List(
	ctx context.Context, project, path string, depth, limit int,
) ([]storage.FileInfo, bool, error) {
	var value []storage.FileInfo
	var more bool
	err := engine.readView(ctx, func(backend storage.Engine) (err error) {
		value, more, err = backend.List(ctx, project, path, depth, limit)
		return err
	})
	return value, more, err
}

// Search applies the local storage operation to the active transaction view.
func (view *transactionView) Search(
	ctx context.Context, project, query, path string, limit int,
) ([]storage.FileChunk, error) {
	if err := view.check(project); err != nil {
		return nil, err
	}
	return view.Engine.Search(ctx, project, query, path, limit)
}

// Search performs the local storage operation under the shared filesystem lock.
func (engine *Engine) Search(ctx context.Context, project, query, path string, limit int) ([]storage.FileChunk, error) {
	var value []storage.FileChunk
	err := engine.readView(ctx, func(backend storage.Engine) (err error) {
		value, err = backend.Search(ctx, project, query, path, limit)
		return err
	})
	return value, err
}

// Delete applies the local storage operation to the active transaction view.
func (view *transactionView) Delete(ctx context.Context, project, path string, recursive bool) error {
	if err := view.check(project); err != nil {
		view.failure = errors.Join(view.failure, err)
		return err
	}
	err := view.Engine.Delete(ctx, project, path, recursive)
	view.failure = errors.Join(view.failure, err)
	return err
}

// Delete performs the local storage operation under the shared filesystem lock.
func (engine *Engine) Delete(ctx context.Context, project, path string, recursive bool) error {
	return engine.Transaction(ctx, project, "", func(backend storage.Engine) error {
		return backend.Delete(ctx, project, path, recursive)
	})
}
