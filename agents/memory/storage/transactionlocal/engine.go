//go:build linux

// Package transactionlocal provides opt-in durable local memory transactions.
package transactionlocal

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"golang.org/x/sys/unix"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// Engine serializes transactions across processes and publishes durable snapshots.
// Its root has a private generation layout; it is not a local.Engine root.
type Engine struct {
	conf local.Config
	root string
}

var _ storage.TransactionalEngine = (*Engine)(nil)

// NewEngine opens a dedicated transaction root using local storage configuration.
// The root must be private to cooperating transactionlocal users on a local
// filesystem supporting flock, atomic rename, and file/directory fsync.
func NewEngine(conf local.Config) (*Engine, error) {
	if strings.TrimSpace(conf.RootDir) == "" {
		return nil, errors.New("root_dir is required")
	}
	root, err := filepath.Abs(conf.RootDir)
	if err != nil {
		return nil, errors.Wrap(err, "resolve root")
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		return nil, errors.Wrap(err, "create root")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, errors.Wrap(err, "resolve root symlinks")
	}
	conf.RootDir = root
	validationBackend, err := local.NewEngine(conf)
	if err != nil {
		return nil, errors.Wrap(err, "validate local configuration")
	}
	if err = validationBackend.Close(); err != nil {
		return nil, errors.Wrap(err, "close configuration validation")
	}
	engine := &Engine{conf: conf, root: root}
	// A dedicated layout avoids accidentally reinterpreting legacy storage.
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, errors.Wrap(err, "read root")
	}
	for _, entry := range entries {
		if entry.Name() != "lock" && entry.Name() != "current" &&
			!strings.HasPrefix(entry.Name(), "generation-") && !strings.HasPrefix(entry.Name(), "pointer-") {
			return nil, errors.New("transaction root contains non-transactional entries; use a dedicated directory")
		}
	}
	// Persist root creation as well as the commit entries below it.
	for directory := root; ; directory = filepath.Dir(directory) {
		if err = syncDirectory(directory); err != nil {
			return nil, err
		}
		if filepath.Dir(directory) == directory {
			break
		}
	}
	return engine, nil
}

// acquire obtains the process-wide filesystem lock, honoring cancellation.
func (engine *Engine) acquire(ctx context.Context) (*os.File, error) {
	fd, err := unix.Open(
		filepath.Join(engine.root, "lock"),
		unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600,
	)
	if err != nil {
		return nil, errors.Wrap(err, "open transaction lock")
	}
	file := os.NewFile(uintptr(fd), "transaction lock")
	for {
		if err = ctx.Err(); err != nil {
			_ = file.Close()
			return nil, errors.Wrap(err, "wait for transaction lock")
		}
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = file.Close()
			return nil, errors.Wrap(err, "lock transaction root")
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

// current resolves the published generation, or returns empty for a new root.
func (engine *Engine) current() (current string, result error) {
	root, err := os.OpenRoot(engine.root)
	if err != nil {
		return "", errors.Wrap(err, "open transaction root")
	}
	defer func() { result = errors.Join(result, errors.Wrap(root.Close(), "close transaction root")) }()
	pointer, err := root.Open("current")
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.Wrap(err, "open generation pointer")
	}
	info, statErr := pointer.Stat()
	if statErr != nil {
		return "", errors.Join(errors.Wrap(statErr, "stat pointer"), errors.Wrap(pointer.Close(), "close pointer"))
	}
	if !info.Mode().IsRegular() || info.Size() > 255 {
		return "", errors.Join(
			errors.New("invalid generation pointer file"), errors.Wrap(pointer.Close(), "close pointer"),
		)
	}
	body, readErr := io.ReadAll(io.LimitReader(pointer, 256))
	if err = errors.Join(
		errors.Wrap(readErr, "read generation pointer"), errors.Wrap(pointer.Close(), "close pointer"),
	); err != nil {
		return "", err
	}
	name := string(body)
	if !strings.HasPrefix(name, "generation-") || filepath.Base(name) != name ||
		strings.ContainsAny(name, "/\\\x00\r\n") {
		return "", errors.New("invalid generation pointer")
	}
	info, err = root.Lstat(name)
	if err != nil {
		return "", errors.Wrap(err, "stat generation")
	}
	if !info.IsDir() {
		return "", errors.New("generation is not a directory")
	}
	return filepath.Join(engine.root, name), nil
}

// Transaction applies fn atomically in a private snapshot and returns its error.
// All processes and instances using this root share one serialization lock.
// Callback errors and cancellation before publication leave current unchanged.
// Errors after pointer publication can have committed; callers must retry by ID.
func (engine *Engine) Transaction(
	ctx context.Context, project, _ string, fn func(storage.Engine) error,
) (result error) {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	lock, err := engine.acquire(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, errors.Wrap(lock.Close(), "close transaction lock")) }()
	current, err := engine.current()
	if err != nil {
		return err
	}
	staged, err := os.MkdirTemp(engine.root, "generation-")
	if err != nil {
		return errors.Wrap(err, "create generation")
	}
	published := false
	defer func() {
		if !published {
			result = errors.Join(result, errors.Wrap(os.RemoveAll(staged), "discard generation"))
		}
	}()
	if current != "" {
		if err = copyTree(ctx, current, staged); err != nil {
			return err
		}
	}
	conf := engine.conf
	conf.RootDir = staged
	backend, err := local.NewEngine(conf)
	if err != nil {
		return errors.Wrap(err, "open generation")
	}
	view := &transactionView{Engine: backend, project: project, active: true}
	callbackErr := func() (callbackResult error) {
		defer func() {
			view.active = false
			callbackResult = errors.Join(callbackResult, errors.Wrap(backend.Close(), "close generation"))
		}()
		return fn(view)
	}()
	if err = errors.Join(callbackErr, view.failure, errors.Wrap(ctx.Err(), "transaction canceled")); err != nil {
		return err
	}
	if err = syncTree(ctx, staged); err != nil {
		return err
	}
	if err = syncDirectory(engine.root); err != nil {
		return err
	}
	pointer, err := os.CreateTemp(engine.root, "pointer-")
	if err != nil {
		return errors.Wrap(err, "create generation pointer")
	}
	pointerName := pointer.Name()
	defer func() { result = errors.Join(result, removeIfPresent(pointerName)) }()
	_, writeErr := pointer.WriteString(filepath.Base(staged))
	syncErr := pointer.Sync()
	closeErr := pointer.Close()
	if err = errors.Join(
		errors.Wrap(writeErr, "write generation pointer"), errors.Wrap(syncErr, "sync generation pointer"),
		errors.Wrap(closeErr, "close generation pointer"), errors.Wrap(ctx.Err(), "transaction canceled"),
	); err != nil {
		return err
	}
	if err = os.Rename(pointerName, filepath.Join(engine.root, "current")); err != nil {
		return errors.Wrap(err, "publish generation")
	}
	published = true
	if err = syncDirectory(engine.root); err != nil {
		return errors.Wrap(err, "commit durability uncertain")
	}
	// The lock excludes every reader, so superseded and abandoned generations
	// may now be removed without invalidating any in-flight view.
	return engine.collect(filepath.Base(staged))
}

// collect removes unreferenced snapshots after a durable commit.
func (engine *Engine) collect(keep string) error {
	entries, err := os.ReadDir(engine.root)
	if err != nil {
		return errors.Wrap(err, "list obsolete generations")
	}
	for _, entry := range entries {
		if entry.Name() != keep &&
			(strings.HasPrefix(entry.Name(), "generation-") || strings.HasPrefix(entry.Name(), "pointer-")) {
			if err = os.RemoveAll(filepath.Join(engine.root, entry.Name())); err != nil {
				return errors.Wrap(err, "remove obsolete generation")
			}
		}
	}
	return nil
}

// readView runs fn against the locked current generation without publishing.
func (engine *Engine) readView(ctx context.Context, fn func(storage.Engine) error) (result error) {
	lock, err := engine.acquire(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, errors.Wrap(lock.Close(), "close transaction lock")) }()
	current, err := engine.current()
	if err != nil {
		return err
	}
	// An empty root is read through a temporary empty local namespace.
	temporary := false
	if current == "" {
		current, err = os.MkdirTemp(engine.root, "generation-")
		temporary = true
	}
	if err != nil {
		return errors.Wrap(err, "create empty read view")
	}
	if temporary {
		defer func() { result = errors.Join(result, errors.Wrap(os.RemoveAll(current), "remove empty read view")) }()
	}
	conf := engine.conf
	conf.RootDir = current
	backend, err := local.NewEngine(conf)
	if err != nil {
		return errors.Wrap(err, "open read view")
	}
	defer func() { result = errors.Join(result, errors.Wrap(backend.Close(), "close read view")) }()
	return fn(backend)
}
