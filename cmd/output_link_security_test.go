package cmd

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// outputLinkFixture holds a disposable work directory and an outside victim.
type outputLinkFixture struct {
	work    string
	victim  string
	outside string
}

// newOutputLinkFixture creates the work directory, an outside victim holding
// victimContent and the path of a not-yet-existing outside file. It takes the
// test handle and the victim content, and returns the fixture.
func newOutputLinkFixture(t *testing.T, victimContent string) outputLinkFixture {
	t.Helper()
	parent := t.TempDir()
	fixture := outputLinkFixture{
		work:    filepath.Join(parent, "work"),
		victim:  filepath.Join(parent, "victim.txt"),
		outside: filepath.Join(parent, "outside.txt"),
	}
	require.NoError(t, os.Mkdir(fixture.work, 0o700))
	require.NoError(t, os.WriteFile(fixture.victim, []byte(victimContent), 0o600))
	return fixture
}

// plantLink creates link pointing at the victim, or at the absent outside file
// when dangling is true. It takes the test handle, the fixture, the link path and
// the dangling selector; unsupported symlinks skip the test.
func (f outputLinkFixture) plantLink(t *testing.T, link string, dangling bool) {
	t.Helper()
	target := f.victim
	if dangling {
		target = f.outside
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

// requireUntouched asserts that the victim still holds want, that no outside file
// was created and that link is still a symbolic link. It takes the test handle,
// the expected victim content and the link path.
func (f outputLinkFixture) requireUntouched(t *testing.T, want, link string) {
	t.Helper()
	data, err := os.ReadFile(f.victim)
	require.NoError(t, err)
	require.Equal(t, want, string(data))
	_, err = os.Lstat(f.outside)
	require.ErrorIs(t, err, os.ErrNotExist, "no file may be created through a dangling link")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.Equal(t, os.ModeSymlink, info.Mode().Type())
}

// linkModes enumerates existing and dangling link variants.
var linkModes = map[string]bool{"existing": false, "dangling": true}

// TestSortJSONFileRejectsLink verifies that the JSON rewrite does not follow a
// final link to an unrelated file; regression for issue #46.
func TestSortJSONFileRejectsLink(t *testing.T) {
	const victim = `{"b":2,"a":1}`
	fixture := newOutputLinkFixture(t, victim)
	link := filepath.Join(fixture.work, "data.json")
	fixture.plantLink(t, link, false)

	require.Error(t, sortJSONFile(link, false, false, 2, false))
	fixture.requireUntouched(t, victim, link)
}

// TestSortJSONPathSkipsLinks verifies that directory scans rewrite regular JSON
// files but leave linked entries and their targets unchanged; regression for issue #46.
func TestSortJSONPathSkipsLinks(t *testing.T) {
	const victim = `{"b":2,"a":1}`
	for _, recursive := range []bool{false, true} {
		t.Run(map[bool]string{false: "flat", true: "recursive"}[recursive], func(t *testing.T) {
			fixture := newOutputLinkFixture(t, victim)
			link := filepath.Join(fixture.work, "link.json")
			fixture.plantLink(t, link, false)
			regular := filepath.Join(fixture.work, "regular.json")
			require.NoError(t, os.WriteFile(regular, []byte(victim), 0o640))

			require.NoError(t, sortJSONPath(fixture.work, []string{".json"}, recursive, false, false, 2, false))
			fixture.requireUntouched(t, victim, link)
			data, err := os.ReadFile(regular)
			require.NoError(t, err)
			require.Equal(t, "{\n  \"a\": 1,\n  \"b\": 2\n}\n", string(data))
		})
	}
}

// TestSortJSONFilePreservesMode verifies that the safe rewrite keeps the original
// permission bits of a regular file; regression for issue #46.
func TestSortJSONFilePreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "mode.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"b":2,"a":1}`), 0o600))
	require.NoError(t, os.Chmod(path, 0o640))

	require.NoError(t, sortJSONFile(path, false, false, 2, false))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

// writeFaviconSource writes a tiny valid PNG input. It takes the test handle and
// the directory, and returns the input path.
func writeFaviconSource(t *testing.T, dir string) string {
	t.Helper()
	input := filepath.Join(dir, "source.png")
	fp, err := os.Create(input)
	require.NoError(t, err)
	require.NoError(t, png.Encode(fp, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	require.NoError(t, fp.Close())
	return input
}

// TestFaviconOutputLinks verifies that favicon generation never writes through an
// existing or dangling output link, with or without --force; regression for issue #46.
func TestFaviconOutputLinks(t *testing.T) {
	for _, force := range []bool{false, true} {
		for mode, dangling := range linkModes {
			name := map[bool]string{false: "no-force", true: "force"}[force] + "/" + mode
			t.Run(name, func(t *testing.T) {
				fixture := newOutputLinkFixture(t, "KEEP")
				input := writeFaviconSource(t, fixture.work)
				link := filepath.Join(fixture.work, "favicon.png")
				fixture.plantLink(t, link, dangling)

				_, err := generateFaviconFile(imageFaviconOptions{
					Input: input, Output: "favicon.png", Sizes: []int{16}, Format: faviconFormatPNG, Force: force,
				})
				require.Error(t, err)
				fixture.requireUntouched(t, "KEEP", link)
			})
		}
	}
}

// TestFaviconForceReplacesRegular verifies that --force still replaces an ordinary
// regular output file, while no-force refuses it; regression for issue #46.
func TestFaviconForceReplacesRegular(t *testing.T) {
	dir := t.TempDir()
	input := writeFaviconSource(t, dir)
	output := filepath.Join(dir, "favicon.png")
	require.NoError(t, os.WriteFile(output, []byte("old"), 0o600))
	opts := imageFaviconOptions{Input: input, Output: "favicon.png", Sizes: []int{16}, Format: faviconFormatPNG}

	_, err := generateFaviconFile(opts)
	require.Error(t, err)
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "old", string(data))

	opts.Force = true
	got, err := generateFaviconFile(opts)
	require.NoError(t, err)
	require.Equal(t, output, got)
	fp, err := os.Open(output)
	require.NoError(t, err)
	defer fp.Close()
	_, err = png.Decode(fp)
	require.NoError(t, err)
}

// writeRSATestKey writes a PEM private key for signing tests. It takes the test
// handle and directory, and returns the key path.
func writeRSATestKey(t *testing.T, dir string) string {
	t.Helper()
	prikey, err := gcrypto.NewRSAPrikey(gcrypto.RSAPrikeyBits2048)
	require.NoError(t, err)
	pem, err := gcrypto.Prikey2Pem(prikey)
	require.NoError(t, err)
	path := filepath.Join(dir, "prikey.pem")
	require.NoError(t, os.WriteFile(path, pem, 0o600))
	pubPem, err := gcrypto.Pubkey2Pem(&prikey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pubkey.pem"), pubPem, 0o600))
	return path
}

// TestSignFileByRSAOutputLinks verifies that the signature output never follows
// an existing or dangling .sig link; regression for issue #46.
func TestSignFileByRSAOutputLinks(t *testing.T) {
	for mode, dangling := range linkModes {
		t.Run(mode, func(t *testing.T) {
			fixture := newOutputLinkFixture(t, "KEEP")
			key := writeRSATestKey(t, t.TempDir())
			data := filepath.Join(fixture.work, "data.txt")
			require.NoError(t, os.WriteFile(data, []byte("payload"), 0o600))
			link := data + ".sig"
			fixture.plantLink(t, link, dangling)

			require.Error(t, SignFileByRSA(key, data))
			fixture.requireUntouched(t, "KEEP", link)
		})
	}
}

// TestSignFileByRSAReplacesRegularSignature verifies that re-signing still replaces
// an ordinary existing signature file and that it verifies; regression for issue #46.
func TestSignFileByRSAReplacesRegularSignature(t *testing.T) {
	dir := t.TempDir()
	key := writeRSATestKey(t, dir)
	data := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(data, []byte("payload"), 0o600))
	require.NoError(t, os.WriteFile(data+".sig", []byte("stale"), 0o600))

	require.NoError(t, SignFileByRSA(key, data))
	require.NoError(t, VerifyFileByRSA(filepath.Join(dir, "pubkey.pem"), data))
}
