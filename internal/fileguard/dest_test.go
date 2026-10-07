package fileguard

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// errInjected is returned by failing test writers.
var errInjected = errors.New("injected write failure")

// failingWrite writes a partial payload and then fails. It takes the open file and
// returns errInjected or the write error.
func failingWrite(f *os.File) error {
	if _, err := f.WriteString("partial"); err != nil {
		return errors.Wrap(err, "write partial payload")
	}
	return errInjected
}

// TestWriteNewRemovesPartialOutput verifies that a failed exclusive write leaves
// no destination behind and that collisions match fs.ErrExist; regression for issue #46.
func TestWriteNewRemovesPartialOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	err := WriteNew(path, 0, 0o600, failingWrite)
	require.ErrorIs(t, err, errInjected)
	_, err = os.Lstat(path)
	require.ErrorIs(t, err, fs.ErrNotExist)

	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o600))
	err = WriteNew(path, 0, 0o600, failingWrite)
	require.ErrorIs(t, err, fs.ErrExist)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "existing", string(data))
}

// TestReplaceFailureKeepsDestination verifies that a failed replacement keeps the
// previous file and removes its private temporary file; regression for issue #46.
func TestReplaceFailureKeepsDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	require.NoError(t, os.WriteFile(path, []byte("previous"), 0o600))

	require.ErrorIs(t, Replace(path, 0, 0o600, failingWrite), errInjected)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "previous", string(data))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the temporary file must be removed")
}

// TestReplaceRejectsSpecialEntries verifies that links and directories are never
// replaced or written through; regression for issue #46.
func TestReplaceRejectsSpecialEntries(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("KEEP"), 0o600))
	link := filepath.Join(dir, "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o700))
	write := func(f *os.File) error {
		_, err := f.WriteString("NEW")
		return errors.Wrap(err, "write")
	}

	for _, path := range []string{link, sub} {
		require.ErrorIs(t, Replace(path, 0, 0o600, write), ErrNotRegular, path)
	}
	data, err := os.ReadFile(victim)
	require.NoError(t, err)
	require.Equal(t, "KEEP", string(data))
}

// TestOpenAppendCreatesAndAppends verifies the ordinary create-then-append path of
// OpenAppend; regression for issue #46.
func TestOpenAppendCreatesAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	for _, line := range []string{"one\n", "two\n"} {
		f, err := OpenAppend(path, 0o600)
		require.NoError(t, err)
		_, err = f.WriteString(line)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "one\ntwo\n", string(data))
}
