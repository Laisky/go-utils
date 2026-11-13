//go:build linux

package transactionlocal

import (
	"context"
	"os"

	"github.com/Laisky/errors/v2"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

var _ storage.ReadTransactionalEngine = (*Engine)(nil)

// transactionView binds a synchronous callback to one project and snapshot.
// Its backend is private: callers cannot bypass lifetime or mutation checks.
type transactionView struct {
	owner    *Engine
	backend  *local.Engine
	project  string
	current  string
	staged   string
	empty    string
	active   bool
	readOnly bool
	failure  error
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

// storage opens the current snapshot, staging a full private copy only for mutations.
func (view *transactionView) storage(ctx context.Context, mutation bool) (storage.Engine, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "transaction operation canceled")
	}
	if mutation && view.readOnly {
		return nil, errors.New("read transaction cannot mutate storage")
	}
	if mutation && view.staged == "" {
		staged, err := os.MkdirTemp(view.owner.root, "generation-")
		if err != nil {
			return nil, errors.Wrap(err, "create generation")
		}
		view.staged = staged
		if view.current != "" {
			if err = copyTree(ctx, view.current, staged); err != nil {
				return nil, err
			}
		}
		if view.backend != nil {
			closeErr := view.backend.Close()
			view.backend = nil
			if closeErr != nil {
				return nil, errors.Wrap(closeErr, "close original snapshot")
			}
		}
	}
	if view.backend != nil {
		return view.backend, nil
	}
	root := view.current
	if view.staged != "" {
		root = view.staged
	}
	if root == "" {
		// Preserve the local adapter's exact empty/project/path semantics without
		// exposing the transaction root's lock and pointer files as project data.
		empty, err := os.MkdirTemp(view.owner.root, "generation-empty-")
		if err != nil {
			return nil, errors.Wrap(err, "create empty read view")
		}
		view.empty = empty
		root = empty
	}
	conf := view.owner.conf
	conf.RootDir = root
	backend, err := local.NewEngine(conf)
	if err != nil {
		return nil, errors.Wrap(err, "open snapshot")
	}
	view.backend = backend
	return backend, nil
}

// close expires the view and closes temporary handles without publishing.
func (view *transactionView) close() error {
	view.active = false
	var result error
	if view.backend != nil {
		result = errors.Wrap(view.backend.Close(), "close snapshot")
		view.backend = nil
	}
	if view.empty != "" {
		result = errors.Join(result, errors.Wrap(removeOwnedSnapshot(view.empty), "remove empty read view"))
		view.empty = ""
	}
	return result
}
