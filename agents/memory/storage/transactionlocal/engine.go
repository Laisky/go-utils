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
	conf         local.Config
	root         string
	rootIdentity os.FileInfo
	// commitSync is an internal filesystem-fault seam; nil uses real directory fsync.
	commitSync func(string) error
	// retireSnapshot is an internal post-commit cleanup fault seam.
	retireSnapshot func(string) error
}

var _ storage.TransactionalEngine = (*Engine)(nil)

// NewEngine opens a dedicated transaction root using local storage configuration.
// The root must be private to cooperating transactionlocal users on a local
// filesystem supporting flock, atomic rename, and file/directory fsync.
func NewEngine(conf local.Config) (engine *Engine, result error) {
	if strings.TrimSpace(conf.RootDir) == "" {
		return nil, errors.New("root_dir is required")
	}
	root, err := filepath.Abs(conf.RootDir)
	if err != nil {
		return nil, errors.Wrap(err, "resolve root")
	}
	created, err := makeRootDirectories(root)
	defer func() {
		if result != nil {
			result = errors.Join(result, removeCreatedDirectories(created))
		}
	}()
	if err != nil {
		return nil, errors.Wrap(err, "create root")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, errors.Wrap(err, "resolve root symlinks")
	}
	identity, err := validateRoot(root, nil)
	if err != nil {
		return nil, err
	}
	conf.RootDir = root
	validationBackend, err := local.NewEngine(conf)
	if err != nil {
		return nil, errors.Wrap(err, "validate local configuration")
	}
	if err = validationBackend.Close(); err != nil {
		return nil, errors.Wrap(err, "close configuration validation")
	}
	engine = &Engine{conf: conf, root: root, rootIdentity: identity}
	if err = syncCreatedDirectories(created, syncDirectory); err != nil {
		return nil, err
	}
	if err = engine.validateLayout(); err != nil {
		return nil, err
	}
	return engine, nil
}

// acquire obtains the exclusive filesystem lock, honoring cancellation.
func (engine *Engine) acquire(ctx context.Context) (*os.File, error) {
	return engine.acquireMode(ctx, unix.LOCK_EX)
}

// acquireShared obtains a shared filesystem lock for an immutable read snapshot.
func (engine *Engine) acquireShared(ctx context.Context) (*os.File, error) {
	return engine.acquireMode(ctx, unix.LOCK_SH)
}

// acquireMode validates the root before opening its lock and checks the opened
// inode before and after waiting. Trusted owners must not mutate this namespace
// concurrently outside the transaction protocol.
func (engine *Engine) acquireMode(ctx context.Context, mode int) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "wait for transaction lock")
	}
	if _, err := validateRoot(engine.root, engine.rootIdentity); err != nil {
		return nil, err
	}
	fd, err := unix.Open(
		filepath.Join(engine.root, "lock"),
		unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600,
	)
	if err != nil {
		return nil, errors.Wrap(err, "open transaction lock")
	}
	file := os.NewFile(uintptr(fd), "transaction lock")
	fail := func(err error) (*os.File, error) {
		return nil, errors.Join(err, errors.Wrap(file.Close(), "close transaction lock"))
	}
	if err := engine.validateLock(file); err != nil {
		return fail(err)
	}
	for {
		if err = ctx.Err(); err != nil {
			return fail(errors.Wrap(err, "wait for transaction lock"))
		}
		err = unix.Flock(fd, mode|unix.LOCK_NB)
		if err == nil {
			if _, err = validateRoot(engine.root, engine.rootIdentity); err != nil {
				return fail(err)
			}
			if err = engine.validateLock(file); err != nil {
				return fail(err)
			}
			return file, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fail(errors.Wrap(err, "lock transaction root"))
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
	linked, err := root.Lstat("current")
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.Wrap(err, "stat generation pointer")
	}
	if err = validateControlEntry(linked, "generation pointer", false); err != nil {
		return "", err
	}
	pointer, err := root.OpenFile("current", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return "", errors.Wrap(err, "open generation pointer")
	}
	info, statErr := pointer.Stat()
	if statErr != nil {
		return "", errors.Join(errors.Wrap(statErr, "stat pointer"), errors.Wrap(pointer.Close(), "close pointer"))
	}
	if !os.SameFile(linked, info) {
		return "", errors.Join(
			errors.New("generation pointer identity changed"), errors.Wrap(pointer.Close(), "close pointer"),
		)
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
	if err = validateControlEntry(info, "generation", true); err != nil {
		return "", err
	}
	return filepath.Join(engine.root, name), nil
}

// Transaction applies fn atomically in a private snapshot and returns its error.
// All processes and instances using this root share one serialization lock.
// Callback errors and cancellation before publication leave current unchanged.
// Post-publication failures are classified with storage.CommitError; callers
// must inspect the outcome and their operation replay semantics before retrying.
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
	published, durable := false, false
	defer func() {
		result = errors.Join(result, errors.Wrap(lock.Close(), "close transaction lock"))
		if published && result != nil {
			outcome := storage.CommitOutcomeUncertain
			if durable {
				outcome = storage.CommitOutcomeDurable
			}
			result = storage.NewCommitError(result, outcome)
		}
	}()
	current, err := engine.current()
	if err != nil {
		return err
	}
	view := &transactionView{owner: engine, project: project, current: current, active: true}
	defer func() {
		result = errors.Join(result, view.close())
		if !published && view.staged != "" {
			result = errors.Join(result, errors.Wrap(removeOwnedSnapshot(view.staged), "discard generation"))
		}
	}()
	callbackErr := func() (callbackResult error) {
		defer func() { callbackResult = errors.Join(callbackResult, view.close()) }()
		return fn(view)
	}()
	if err = errors.Join(callbackErr, view.failure, errors.Wrap(ctx.Err(), "transaction canceled")); err != nil {
		return err
	}
	if view.staged == "" {
		return nil
	}
	staged := view.staged
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
	if err = engine.syncPublishedDirectory(); err != nil {
		return errors.Wrap(err, "commit durability uncertain")
	}
	durable = true
	// The lock excludes every reader, so superseded and abandoned generations
	// may now be removed without invalidating any in-flight view.
	return engine.collect(current, staged)
}

// ReadTransaction supplies a stable shared snapshot and rejects all mutations.
func (engine *Engine) ReadTransaction(
	ctx context.Context, project, _ string, fn func(storage.Engine) error,
) error {
	return engine.readView(ctx, project, fn)
}

// readView runs fn against a shared current snapshot without staging or publication.
func (engine *Engine) readView(
	ctx context.Context, project string, fn func(storage.Engine) error,
) (result error) {
	if fn == nil {
		return errors.New("read transaction callback is required")
	}
	lock, err := engine.acquireShared(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, errors.Wrap(lock.Close(), "close read transaction lock")) }()
	current, err := engine.current()
	if err != nil {
		return err
	}
	view := &transactionView{owner: engine, project: project, current: current, active: true, readOnly: true}
	defer func() { result = errors.Join(result, view.close()) }()
	result = fn(view)
	return errors.Join(result, view.failure, errors.Wrap(ctx.Err(), "read transaction canceled"))
}
