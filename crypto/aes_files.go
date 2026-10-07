package crypto

import (
	"context"
	"crypto/rand"
	"io"
	"io/fs"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-utils/v6/internal/fileguard"
	"github.com/Laisky/go-utils/v6/log"
)

const (
	defaultEncryptSuffix = ".enc"

	// DefaultAESFilesInDirMaxFiles is the default maximum number of files one
	// directory run may encrypt.
	DefaultAESFilesInDirMaxFiles = 10000
	// DefaultAESFilesInDirMaxFileBytes is the default per-file plaintext budget (64 MiB).
	DefaultAESFilesInDirMaxFileBytes int64 = 64 << 20
	// DefaultAESFilesInDirMaxTotalBytes is the default aggregate plaintext budget (1 GiB).
	DefaultAESFilesInDirMaxTotalBytes int64 = 1 << 30
	// defaultAESFilesInDirMaxConcurrency caps the default worker count.
	defaultAESFilesInDirMaxConcurrency = 4
	// maxAESFilesInDirConcurrency is the largest accepted worker count.
	maxAESFilesInDirConcurrency = 256
	// aesFilesTempPrefix names private staging files; such entries are never inputs.
	aesFilesTempPrefix = ".go-utils-aes-"
	// aesFilesReadDirBatch bounds directory entries held per ReadDir call.
	aesFilesReadDirBatch = 256
)

// aesFilesHookStage identifies where the package-test hook runs.
type aesFilesHookStage int

const (
	// aesFilesHookBeforeOpen runs after scheduling, before the source is opened.
	aesFilesHookBeforeOpen aesFilesHookStage = iota
	// aesFilesHookBeforeRead runs after the opened descriptor passed its size check.
	aesFilesHookBeforeRead
)

// encryptFilesOption holds AESEncryptFilesInDir configuration.
type encryptFilesOption struct {
	ext string
	// suffix will append in encrypted file'name after ext as suffix
	suffix         string
	maxConcurrency int
	maxFiles       int
	maxFileBytes   int64
	maxTotalBytes  int64
	// hook is a package-test instrumentation point; it is nil in production.
	hook func(ctx context.Context, stage aesFilesHookStage, name string) error
}

// fillDefault sets the documented finite defaults.
func (o *encryptFilesOption) fillDefault() {
	o.suffix = defaultEncryptSuffix
	o.maxConcurrency = min(defaultAESFilesInDirMaxConcurrency, max(1, runtime.GOMAXPROCS(0)))
	o.maxFiles = DefaultAESFilesInDirMaxFiles
	o.maxFileBytes = DefaultAESFilesInDirMaxFileBytes
	o.maxTotalBytes = DefaultAESFilesInDirMaxTotalBytes
}

// AESEncryptFilesInDirOption options to encrypt files in dir
type AESEncryptFilesInDirOption func(*encryptFilesOption) error

// WithAESFilesInDirFileExt only encrypt files with specific ext
func WithAESFilesInDirFileExt(ext string) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if !strings.HasPrefix(ext, ".") {
			return errors.Errorf("ext should start with `.`")
		}

		opt.ext = ext
		return nil
	}
}

// WithAESFilesInDirFileSuffix will append to encrypted's filename as suffix
//
//	xxx.toml -> xxx.toml.enc
//
// The suffix must start with "." and must not contain path separators or NUL.
func WithAESFilesInDirFileSuffix(suffix string) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if !strings.HasPrefix(suffix, ".") {
			return errors.Errorf("suffix should start with `.`")
		}
		if strings.ContainsAny(suffix, "/\\\x00") {
			return errors.Errorf("suffix must not contain path separators")
		}

		opt.suffix = suffix
		return nil
	}
}

// WithAESFilesInDirMaxConcurrency sets the maximum number of files processed
// at once (default min(4, GOMAXPROCS)). It must be within [1, 256]; the returned
// option fails otherwise. Peak plaintext memory is roughly n * max file bytes * 2.
func WithAESFilesInDirMaxConcurrency(n int) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if n < 1 || n > maxAESFilesInDirConcurrency {
			return errors.Errorf("max concurrency must be within [1, %d], got %d", maxAESFilesInDirConcurrency, n)
		}

		opt.maxConcurrency = n
		return nil
	}
}

// WithAESFilesInDirMaxFiles sets the maximum number of matching files
// (default DefaultAESFilesInDirMaxFiles). It must be positive; a directory with
// more matching files is rejected before any output is written.
func WithAESFilesInDirMaxFiles(n int) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if n < 1 {
			return errors.Errorf("max files must be positive, got %d", n)
		}

		opt.maxFiles = n
		return nil
	}
}

// WithAESFilesInDirMaxFileBytes sets the per-file plaintext budget (default
// DefaultAESFilesInDirMaxFileBytes). It must be positive and no larger than the
// AES-GCM single-message limit; the returned option fails otherwise.
func WithAESFilesInDirMaxFileBytes(n int64) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if n < 1 || n > maxGCMPlaintextLen {
			return errors.Errorf("max file bytes must be within [1, %d], got %d", int64(maxGCMPlaintextLen), n)
		}

		opt.maxFileBytes = n
		return nil
	}
}

// WithAESFilesInDirMaxTotalBytes sets the aggregate plaintext budget for one
// run (default DefaultAESFilesInDirMaxTotalBytes). It must be positive.
func WithAESFilesInDirMaxTotalBytes(n int64) AESEncryptFilesInDirOption {
	return func(opt *encryptFilesOption) error {
		if n < 1 {
			return errors.Errorf("max total bytes must be positive, got %d", n)
		}

		opt.maxTotalBytes = n
		return nil
	}
}

// AESEncryptFilesInDir encrypts the regular files directly inside dir with the
// raw AES key secret, using the AesEncrypt (AES-GCM) format. It is
// AESEncryptFilesInDirWithContext with context.Background().
//
//	xxx.toml -> xxx.toml.enc
func AESEncryptFilesInDir(dir string, secret []byte, opts ...AESEncryptFilesInDirOption) (err error) {
	return AESEncryptFilesInDirWithContext(context.Background(), dir, secret, opts...)
}

// AESEncryptFilesInDirWithContext encrypts the regular files directly inside dir
// (not recursive) with the raw 16/24/32-byte AES key secret, writing
// <name><suffix> next to each source in the AesEncrypt/AEADEncrypt format.
//
// Resource bounds: at most max-concurrency files are processed at once, and the
// run is rejected when it matches more than max files, any file exceeds max
// file bytes, or the matched files exceed max total bytes. Budgets are checked
// during enumeration (before any output is written), again on the opened
// descriptor, and while reading, so files that grow concurrently are rejected.
//
// Skipped entries: directories, symlinks, FIFOs and other non-regular files;
// names that already end with the suffix; names that do not end with the
// selected extension; and private staging files of this function.
//
// Outputs: each output is staged in a private, exclusively created temporary
// file in dir and atomically renamed over <name><suffix>. An existing regular
// output is replaced; an existing symlink or special file at the output name is
// never followed or replaced and fails the run. On error or cancellation no
// further files are scheduled and staging files are removed, but outputs that
// were already published remain. The directory path itself is trusted.
func AESEncryptFilesInDirWithContext(ctx context.Context, dir string, secret []byte,
	opts ...AESEncryptFilesInDirOption) error {
	if err := checkAESKeyLen(secret); err != nil {
		return errors.WithStack(err)
	}

	key := slices.Clone(secret)
	return encryptFilesInDir(ctx, dir, func(plaintext []byte) ([]byte, error) {
		return AesEncrypt(key, plaintext)
	}, opts...)
}

// PasswordEncryptFilesInDirWithContext is AESEncryptFilesInDirWithContext for
// password mode: every file is sealed by enc, so the Argon2id derivation is paid
// once per run while each output stays self-describing (header, salt and a
// fresh nonce) and is decryptable by DecryptByPassword. Empty files are allowed.
func PasswordEncryptFilesInDirWithContext(ctx context.Context, dir string, enc *PasswordEncryptor,
	opts ...AESEncryptFilesInDirOption) error {
	if enc == nil {
		return errors.New("password encryptor must not be nil")
	}

	return encryptFilesInDir(ctx, dir, enc.Encrypt, opts...)
}

// aesFilesCandidate is a matched source entry and its enumeration metadata.
type aesFilesCandidate struct {
	name string
	info os.FileInfo
}

// aesFilesRun carries the shared state of one directory run.
type aesFilesRun struct {
	root    *os.Root
	opt     *encryptFilesOption
	encrypt func([]byte) ([]byte, error)
	total   atomic.Int64
	logger  *log.LoggerT
}

// encryptFilesInDir applies options, enumerates candidates within budgets, and
// encrypts them with a bounded worker pool. It returns the first error.
func encryptFilesInDir(ctx context.Context, dir string, encrypt func([]byte) ([]byte, error),
	opts ...AESEncryptFilesInDirOption) (retErr error) {
	if ctx == nil {
		return errors.New("context must not be nil")
	}

	opt := new(encryptFilesOption)
	opt.fillDefault()
	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return errors.Wrap(err, "apply AES encrypt files option")
		}
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "encrypt files in dir")
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return errors.Wrapf(err, "open dir `%s`", dir)
	}
	defer func() { retErr = errors.Join(retErr, errors.Wrap(root.Close(), "close dir")) }()

	run := &aesFilesRun{root: root, opt: opt, encrypt: encrypt, logger: log.Shared.With(
		zap.String("dir", dir), zap.String("ext", opt.ext), zap.String("suffix", opt.suffix))}

	candidates, err := run.enumerate(ctx)
	if err != nil {
		return errors.Wrapf(err, "enumerate dir `%s`", dir)
	}
	run.logger.Debug("encrypt files in dir",
		zap.Int("files", len(candidates)), zap.Int("concurrency", opt.maxConcurrency))

	pool, gctx := errgroup.WithContext(ctx)
	pool.SetLimit(opt.maxConcurrency)
	for _, c := range candidates {
		if gctx.Err() != nil {
			break // stop scheduling after the first failure or cancellation
		}
		pool.Go(func() error {
			return errors.Wrapf(run.encryptOne(gctx, c), "encrypt file `%s`", c.name)
		})
	}

	if err := pool.Wait(); err != nil {
		return errors.WithStack(err)
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "encrypt files in dir")
	}

	return nil
}

// enumerate streams directory entries in batches and returns the sorted
// matching regular files. It fails as soon as the file-count, per-file or
// aggregate budget is exceeded, so no unbounded list is materialized.
func (r *aesFilesRun) enumerate(ctx context.Context) (candidates []aesFilesCandidate, retErr error) {
	d, err := r.root.Open(".")
	if err != nil {
		return nil, errors.Wrap(err, "open dir for listing")
	}
	defer func() { retErr = errors.Join(retErr, errors.Wrap(d.Close(), "close dir listing")) }()

	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.WithStack(err)
		}

		entries, readErr := d.ReadDir(aesFilesReadDirBatch)
		for _, entry := range entries {
			c, ok, err := r.match(entry)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if !ok {
				continue
			}

			if len(candidates) >= r.opt.maxFiles {
				return nil, errors.Errorf("directory has more than %d matching files", r.opt.maxFiles)
			}
			if total += c.info.Size(); total > r.opt.maxTotalBytes {
				return nil, errors.Errorf("matching files exceed the aggregate budget of %d bytes", r.opt.maxTotalBytes)
			}
			candidates = append(candidates, c)
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, errors.Wrap(readErr, "read dir")
		}
	}

	slices.SortFunc(candidates, func(a, b aesFilesCandidate) int { return strings.Compare(a.name, b.name) })
	return candidates, nil
}

// match reports whether entry is an input candidate. It returns an error only
// for a regular candidate that exceeds the per-file budget or cannot be inspected.
func (r *aesFilesRun) match(entry fs.DirEntry) (aesFilesCandidate, bool, error) {
	name := entry.Name()
	switch {
	case !entry.Type().IsRegular():
		r.logger.Debug("skip non-regular entry", zap.String("name", name))
		return aesFilesCandidate{}, false, nil
	case !strings.HasSuffix(name, r.opt.ext),
		strings.HasSuffix(name, r.opt.suffix),
		strings.HasPrefix(name, aesFilesTempPrefix):
		return aesFilesCandidate{}, false, nil
	}

	info, err := entry.Info()
	if err != nil {
		return aesFilesCandidate{}, false, errors.Wrapf(err, "inspect `%s`", name)
	}
	if !info.Mode().IsRegular() {
		return aesFilesCandidate{}, false, nil
	}
	if info.Size() > r.opt.maxFileBytes {
		return aesFilesCandidate{}, false, errors.Errorf("file `%s` exceeds the per-file budget of %d bytes",
			name, r.opt.maxFileBytes)
	}

	return aesFilesCandidate{name: name, info: info}, true, nil
}

// encryptOne reads one source under the budgets, encrypts it, and publishes
// the output. It returns the first error encountered.
func (r *aesFilesRun) encryptOne(ctx context.Context, c aesFilesCandidate) error {
	if err := r.runHook(ctx, aesFilesHookBeforeOpen, c.name); err != nil {
		return errors.WithStack(err)
	}

	raw, err := r.readBounded(ctx, c)
	if err != nil {
		return errors.WithStack(err)
	}

	ciphertext, err := r.encrypt(raw)
	clear(raw)
	if err != nil {
		return errors.Wrap(err, "encrypt")
	}

	outName := c.name + r.opt.suffix
	if err := publishInRoot(ctx, r.root, outName, ciphertext); err != nil {
		return errors.Wrapf(err, "publish `%s`", outName)
	}

	r.logger.Info("encrypt file", zap.String("src", c.name), zap.String("out", outName))
	return nil
}

// runHook checks cancellation and invokes the package-test hook when set.
func (r *aesFilesRun) runHook(ctx context.Context, stage aesFilesHookStage, name string) error {
	if err := ctx.Err(); err != nil {
		return errors.WithStack(err)
	}
	if r.opt.hook == nil {
		return nil
	}

	return errors.WithStack(r.opt.hook(ctx, stage, name))
}

// readBounded opens the inspected source without following links or blocking on
// FIFOs, enforces the per-file and aggregate budgets on the descriptor and
// during the read, and returns the plaintext. Cancellation closes the file.
func (r *aesFilesRun) readBounded(ctx context.Context, c aesFilesCandidate) (raw []byte, retErr error) {
	f, err := fileguard.OpenRegular(r.root, c.name, c.info)
	if err != nil {
		return nil, errors.Wrap(err, "open source")
	}
	closeFile := sync.OnceValue(f.Close)
	// The close result is not dropped: sync.OnceValue caches it, and the deferred
	// call below returns the same error and joins it into retErr.
	stop := context.AfterFunc(ctx, func() { _ = closeFile() })
	defer func() {
		stop()
		retErr = errors.Join(retErr, errors.Wrap(closeFile(), "close source"))
	}()

	st, err := f.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "stat source")
	}
	size := st.Size()
	if size > r.opt.maxFileBytes {
		return nil, errors.Errorf("file exceeds the per-file budget of %d bytes", r.opt.maxFileBytes)
	}
	if r.total.Add(size) > r.opt.maxTotalBytes {
		return nil, errors.Errorf("files exceed the aggregate budget of %d bytes", r.opt.maxTotalBytes)
	}
	if err := r.runHook(ctx, aesFilesHookBeforeRead, c.name); err != nil {
		return nil, errors.WithStack(err)
	}

	raw, err = io.ReadAll(io.LimitReader(f, r.opt.maxFileBytes+1))
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, errors.WithStack(ctxErr)
	}
	if err != nil {
		return nil, errors.Wrap(err, "read source")
	}
	if int64(len(raw)) > r.opt.maxFileBytes {
		return nil, errors.Errorf("file grew beyond the per-file budget of %d bytes", r.opt.maxFileBytes)
	}
	if grown := int64(len(raw)) - size; grown > 0 && r.total.Add(grown) > r.opt.maxTotalBytes {
		return nil, errors.Errorf("files grew beyond the aggregate budget of %d bytes", r.opt.maxTotalBytes)
	}

	return raw, nil
}

// publishInRoot writes content to a private, exclusively created staging file
// inside root and renames it over name. An existing name that is not a regular
// file (for example a symlink, even a dangling one) is rejected and left
// untouched. The staging file is removed on any failure.
func publishInRoot(ctx context.Context, root *os.Root, name string, content []byte) (retErr error) {
	if old, err := root.Lstat(name); err == nil && !old.Mode().IsRegular() {
		return errors.New("output exists and is not a regular file (symlink or special file)")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Wrap(err, "inspect output")
	}

	staging := aesFilesTempPrefix + rand.Text()
	f, err := root.OpenFile(staging, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.Wrap(err, "create staging file")
	}
	closed, published := false, false
	defer func() {
		if !closed {
			retErr = errors.Join(retErr, errors.Wrap(f.Close(), "close staging file"))
		}
		if !published {
			retErr = errors.Join(retErr, errors.Wrap(root.Remove(staging), "remove staging file"))
		}
	}()

	if _, err := f.Write(content); err != nil {
		return errors.Wrap(err, "write staging file")
	}
	if err := f.Sync(); err != nil {
		return errors.Wrap(err, "sync staging file")
	}
	closed = true
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close staging file")
	}
	if err := ctx.Err(); err != nil {
		return errors.WithStack(err)
	}
	// Re-check right before publishing; rename never follows a final link, so a
	// link planted after this check is replaced rather than written through.
	if old, err := root.Lstat(name); err == nil && !old.Mode().IsRegular() {
		return errors.New("output exists and is not a regular file (symlink or special file)")
	}
	if err := root.Rename(staging, name); err != nil {
		return errors.Wrap(err, "rename staging file")
	}
	published = true

	return nil
}
