package cmd

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// aesCLITestPassword is a synthetic, human-readable 16-byte test password.
const aesCLITestPassword = "testpassword1234"

// aesCLITestKey is a synthetic raw AES-128 key used by key-file tests.
var aesCLITestKey = []byte("0123456789abcdef")

// setupAESCLITest resets the global cobra state of the AES commands, lowers the
// Argon2id cost to the accepted floor for speed, and restores everything when
// the test ends. AES CLI tests must not run in parallel.
func setupAESCLITest(t *testing.T) {
	t.Helper()
	reset := func() {
		encryptAESArgs, decryptAESArgs = aesCLIArgs{}, aesCLIArgs{}
		for _, f := range []string{"secret", "password-file", "key-file"} {
			if flag := EncryptAESCMD.Flags().Lookup(f); flag != nil {
				flag.Changed = false
			}
		}
		rootCmd.SetIn(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	}
	params, isTerm, readPw := aesCLIPasswordKDFParams, aesIsTerminal, aesReadPassword
	reset()
	aesCLIPasswordKDFParams = gcrypto.PasswordKDFParams{Time: 2, MemoryKiB: 19 * 1024, Parallelism: 1}
	t.Cleanup(func() {
		reset()
		aesCLIPasswordKDFParams, aesIsTerminal, aesReadPassword = params, isTerm, readPw
	})
}

// runAESCLI executes the root command with args (which must never carry
// secrets) and stdin, asserting first that no argument contains the synthetic
// password or key in literal or hex form. It returns the command error.
func runAESCLI(t *testing.T, stdin []byte, args ...string) error {
	t.Helper()
	for _, a := range args {
		for _, secret := range []string{aesCLITestPassword, string(aesCLITestKey), hex.EncodeToString(aesCLITestKey),
			hex.EncodeToString([]byte(aesCLITestPassword))} {
			require.NotContains(t, a, secret, "secret material must never be passed in argv")
		}
	}

	encryptAESArgs, decryptAESArgs = aesCLIArgs{}, aesCLIArgs{}
	rootCmd.SetIn(bytes.NewReader(stdin))
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// writeSecretFile writes content to a new 0600 file in dir and returns its path.
func writeSecretFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	require.NoError(t, os.Chmod(p, 0o600))
	return p
}

// TestAESCLISecurity_SecretFlagRemoved checks that -s/--secret fails closed and
// writes nothing. Regression for issues #44 and #54 (the old CLI used argv and
// the password bytes as the AES key).
func TestAESCLISecurity_SecretFlagRemoved(t *testing.T) {
	setupAESCLITest(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(in, []byte("hello"), 0o600))

	for _, flag := range []string{"-s", "--secret"} {
		encryptAESArgs = aesCLIArgs{}
		rootCmd.SetErr(&bytes.Buffer{})
		rootCmd.SetOut(&bytes.Buffer{})
		rootCmd.SetArgs([]string{"encrypt", "aes", "-i", in, flag, "legacy-value-xx"})
		err := rootCmd.Execute()
		require.ErrorContains(t, err, "--password-file", flag)
		require.ErrorContains(t, err, "--legacy-password-as-key", flag)
		_, err = os.Lstat(in + ".enc")
		require.ErrorIs(t, err, os.ErrNotExist, flag)
		EncryptAESCMD.Flags().Lookup("secret").Changed = false
	}
}

// TestAESCLISecurity_PasswordFileMode checks the Argon2id password format for
// a password file and stdin, and that the password is not the AES key.
// Regression for issues #44 and #54.
func TestAESCLISecurity_PasswordFileMode(t *testing.T) {
	setupAESCLITest(t)
	dir := t.TempDir()
	msg := []byte("hello password mode")
	in := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(in, msg, 0o600))
	pwFile := writeSecretFile(t, dir, "pw.txt", aesCLITestPassword+"\n")

	var outputs [][]byte
	for i, source := range []struct {
		flag  string
		stdin []byte
	}{{pwFile, nil}, {"-", []byte(aesCLITestPassword + "\r\n")}} {
		out := filepath.Join(dir, "out"+string(rune('a'+i))+".enc")
		require.NoError(t, runAESCLI(t, source.stdin, "encrypt", "aes", "-i", in, "-o", out,
			"--password-file", source.flag))

		ct, err := os.ReadFile(out)
		require.NoError(t, err)
		st, err := os.Stat(out)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			require.Equal(t, os.FileMode(0o600), st.Mode().Perm())
		}
		require.True(t, gcrypto.IsPasswordEncrypted(ct))
		_, err = gcrypto.AEADDecrypt([]byte(aesCLITestPassword), ct, nil)
		require.Error(t, err, "password bytes must not be the AES key")
		got, err := gcrypto.DecryptByPassword([]byte(aesCLITestPassword), ct)
		require.NoError(t, err)
		require.Equal(t, msg, got)
		outputs = append(outputs, ct)

		// decrypt aes auto-detects the password format.
		dec := filepath.Join(dir, "dec"+string(rune('a'+i))+".txt")
		require.NoError(t, runAESCLI(t, source.stdin, "decrypt", "aes", "-i", out, "-o", dec,
			"--password-file", source.flag))
		plain, err := os.ReadFile(dec)
		require.NoError(t, err)
		require.Equal(t, msg, plain)
	}
	require.NotEqual(t, outputs[0][:63], outputs[1][:63], "fresh salt and nonce per run")

	wrong := writeSecretFile(t, dir, "wrong.txt", "testpassword1235")
	err := runAESCLI(t, nil, "decrypt", "aes", "-i", filepath.Join(dir, "outa.enc"),
		"-o", filepath.Join(dir, "wrong.out"), "--password-file", wrong)
	require.Error(t, err)
	_, err = os.Lstat(filepath.Join(dir, "wrong.out"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestAESCLISecurity_KeyFileMode checks strict hex raw-key mode and that its
// output stays compatible with AEADDecrypt. Regression for issues #44 and #54.
func TestAESCLISecurity_KeyFileMode(t *testing.T) {
	setupAESCLITest(t)
	dir := t.TempDir()
	msg := []byte("hello raw key mode")
	in := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(in, msg, 0o600))
	keyFile := writeSecretFile(t, dir, "key.hex", "  "+hex.EncodeToString(aesCLITestKey)+"\n")

	require.NoError(t, runAESCLI(t, nil, "encrypt", "aes", "-i", in, "--key-file", keyFile))
	ct, err := os.ReadFile(in + ".enc")
	require.NoError(t, err)
	require.False(t, gcrypto.IsPasswordEncrypted(ct))
	got, err := gcrypto.AEADDecrypt(aesCLITestKey, ct, nil)
	require.NoError(t, err)
	require.Equal(t, msg, got)

	require.NoError(t, os.Remove(in))
	require.NoError(t, runAESCLI(t, []byte(hex.EncodeToString(aesCLITestKey)),
		"decrypt", "aes", "-i", in+".enc", "--key-file", "-"))
	plain, err := os.ReadFile(in)
	require.NoError(t, err)
	require.Equal(t, msg, plain)

	for name, content := range map[string]string{
		"0x prefix":  "0x" + hex.EncodeToString(aesCLITestKey),
		"odd length": hex.EncodeToString(aesCLITestKey)[1:],
		"short":      hex.EncodeToString(aesCLITestKey[:8]),
		"raw bytes":  string(aesCLITestKey),
		"empty":      "",
		"inner sp":   hex.EncodeToString(aesCLITestKey[:8]) + " " + hex.EncodeToString(aesCLITestKey[8:]),
	} {
		bad := writeSecretFile(t, dir, "bad.hex", content)
		err := runAESCLI(t, nil, "encrypt", "aes", "-i", filepath.Join(dir, "in.txt"), "-o",
			filepath.Join(dir, "bad.enc"), "--key-file", bad)
		require.Error(t, err, name)
		if content != "" {
			require.NotContains(t, err.Error(), content, "%s: error must not echo key material", name)
		}
	}

	pw := writeSecretFile(t, dir, "pw.txt", aesCLITestPassword)
	err = runAESCLI(t, nil, "encrypt", "aes", "-i", filepath.Join(dir, "in.txt"),
		"--key-file", keyFile, "--password-file", pw)
	require.ErrorContains(t, err, "only one of")
}

// TestAESCLISecurity_SecretFilePermissions checks that group/world-accessible
// secret files are refused on Unix. Regression for issue #44.
func TestAESCLISecurity_SecretFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	setupAESCLITest(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(in, []byte("x"), 0o600))

	for _, mode := range []os.FileMode{0o640, 0o604, 0o660} {
		for _, flag := range []string{"--password-file", "--key-file"} {
			p := writeSecretFile(t, dir, "secret", hex.EncodeToString(aesCLITestKey))
			require.NoError(t, os.Chmod(p, mode))
			err := runAESCLI(t, nil, "encrypt", "aes", "-i", in, flag, p)
			require.ErrorContains(t, err, "accessible by group or others", "%s %o", flag, mode)
			require.ErrorContains(t, err, "chmod 600")
		}
	}

	err := runAESCLI(t, nil, "encrypt", "aes", "-i", in, "--password-file", dir)
	require.Error(t, err, "a directory is not a secret file")
	err = runAESCLI(t, nil, "encrypt", "aes", "-i", in, "--password-file", filepath.Join(dir, "missing"))
	require.Error(t, err)
	_, err = os.Lstat(in + ".enc")
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestAESCLISecurity_NonInteractiveFailsClosed checks that without a secret
// source and without a terminal the command refuses to run. Regression for
// issue #44.
func TestAESCLISecurity_NonInteractiveFailsClosed(t *testing.T) {
	setupAESCLITest(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(in, []byte("x"), 0o600))

	err := runAESCLI(t, nil, "encrypt", "aes", "-i", in)
	require.ErrorContains(t, err, "no secret input")

	// A real *os.File that is not a terminal also fails closed.
	stdin, err := os.Open(in)
	require.NoError(t, err)
	defer stdin.Close()
	encryptAESArgs = aesCLIArgs{}
	rootCmd.SetIn(stdin)
	rootCmd.SetErr(&bytes.Buffer{})
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetArgs([]string{"encrypt", "aes", "-i", in})
	require.ErrorContains(t, rootCmd.Execute(), "no secret input")

	empty := writeSecretFile(t, dir, "empty.txt", "\n")
	err = runAESCLI(t, nil, "encrypt", "aes", "-i", in, "--password-file", empty)
	require.ErrorContains(t, err, "empty password")
	_, err = os.Lstat(in + ".enc")
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestAESCLISecurity_InteractivePrompt drives the no-echo prompt through a fake
// terminal: encryption requires a matching confirmation. Regression for issue #44.
func TestAESCLISecurity_InteractivePrompt(t *testing.T) {
	setupAESCLITest(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(in, []byte("prompted"), 0o600))
	stdin, err := os.Open(in)
	require.NoError(t, err)
	defer stdin.Close()

	var answers []string
	aesIsTerminal = func(int) bool { return true }
	aesReadPassword = func(int) ([]byte, error) {
		next := answers[0]
		answers = answers[1:]
		return []byte(next), nil
	}
	run := func(args ...string) error {
		encryptAESArgs, decryptAESArgs = aesCLIArgs{}, aesCLIArgs{}
		rootCmd.SetIn(stdin)
		rootCmd.SetErr(&bytes.Buffer{})
		rootCmd.SetOut(&bytes.Buffer{})
		rootCmd.SetArgs(args)
		return rootCmd.Execute()
	}

	answers = []string{aesCLITestPassword, "different"}
	require.ErrorContains(t, run("encrypt", "aes", "-i", in), "do not match")
	_, err = os.Lstat(in + ".enc")
	require.ErrorIs(t, err, os.ErrNotExist)

	answers = []string{aesCLITestPassword, aesCLITestPassword}
	require.NoError(t, run("encrypt", "aes", "-i", in))
	answers = []string{aesCLITestPassword}
	require.NoError(t, run("decrypt", "aes", "-i", in+".enc", "-o", in+".out"))
	got, err := os.ReadFile(in + ".out")
	require.NoError(t, err)
	require.Equal(t, "prompted", string(got))
	require.Empty(t, answers)
}

// TestAESCLISecurity_LegacyDecrypt checks that files from the removed -s flag
// decrypt only with the explicitly named legacy flag. Regression for issue #54.
func TestAESCLISecurity_LegacyDecrypt(t *testing.T) {
	setupAESCLITest(t)
	dir := t.TempDir()
	legacy, err := gcrypto.AEADEncrypt([]byte(aesCLITestPassword), []byte("old data"), nil)
	require.NoError(t, err)
	in := filepath.Join(dir, "old.txt.enc")
	require.NoError(t, os.WriteFile(in, legacy, 0o600))
	pw := writeSecretFile(t, dir, "pw.txt", aesCLITestPassword)

	err = runAESCLI(t, nil, "decrypt", "aes", "-i", in, "--password-file", pw)
	require.ErrorContains(t, err, "--legacy-password-as-key")

	require.NoError(t, runAESCLI(t, nil, "decrypt", "aes", "-i", in, "--password-file", pw,
		"--legacy-password-as-key"))
	got, err := os.ReadFile(filepath.Join(dir, "old.txt"))
	require.NoError(t, err)
	require.Equal(t, "old data", string(got))

	short := writeSecretFile(t, dir, "short.txt", "short")
	err = runAESCLI(t, nil, "decrypt", "aes", "-i", in, "-o", filepath.Join(dir, "x"),
		"--password-file", short, "--legacy-password-as-key")
	require.ErrorContains(t, err, "16, 24 or 32-byte password")
	keyFile := writeSecretFile(t, dir, "key.hex", hex.EncodeToString(aesCLITestKey))
	err = runAESCLI(t, nil, "decrypt", "aes", "-i", in, "--key-file", keyFile, "--legacy-password-as-key")
	require.Error(t, err)
}

// TestAESCLISecurity_OutputLinksUntouched checks that existing and dangling
// output symlinks are refused and left untouched for encrypt and decrypt.
// Regression for issue #46.
func TestAESCLISecurity_OutputLinksUntouched(t *testing.T) {
	setupAESCLITest(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(in, []byte("hello"), 0o600))
	keyFile := writeSecretFile(t, dir, "key.hex", hex.EncodeToString(aesCLITestKey))
	victim := filepath.Join(dir, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("KEEP"), 0o600))
	outside := filepath.Join(dir, "outside.txt")

	require.NoError(t, runAESCLI(t, nil, "encrypt", "aes", "-i", in, "-o", in+".enc", "--key-file", keyFile))
	for _, target := range []string{victim, outside} {
		link := filepath.Join(dir, "link-"+filepath.Base(target))
		require.NoError(t, os.Symlink(target, link))
		for _, args := range [][]string{
			{"encrypt", "aes", "-i", in, "-o", link, "--key-file", keyFile},
			{"decrypt", "aes", "-i", in + ".enc", "-o", link, "--key-file", keyFile},
		} {
			err := runAESCLI(t, nil, args...)
			require.ErrorContains(t, err, "not a regular file", strings.Join(args, " "))
			dest, err := os.Readlink(link)
			require.NoError(t, err)
			require.Equal(t, target, dest)
		}
	}
	content, err := os.ReadFile(victim)
	require.NoError(t, err)
	require.Equal(t, "KEEP", string(content))
	_, err = os.Lstat(outside)
	require.ErrorIs(t, err, os.ErrNotExist)

	// A directory at the output path is refused too; a regular file is replaced.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "d.enc"), 0o700))
	require.Error(t, runAESCLI(t, nil, "encrypt", "aes", "-i", in, "-o", filepath.Join(dir, "d.enc"),
		"--key-file", keyFile))
	require.NoError(t, runAESCLI(t, nil, "encrypt", "aes", "-i", in, "-o", victim, "--key-file", keyFile))
	ct, err := os.ReadFile(victim)
	require.NoError(t, err)
	got, err := gcrypto.AEADDecrypt(aesCLITestKey, ct, nil)
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
}

// TestAESCLISecurity_DirectoryMode checks that directory runs use the same
// secret semantics as file mode: password mode derives once per run (shared
// salt, fresh nonces) and raw-key mode uses the key file. Regression for
// issues #54 and #65.
func TestAESCLISecurity_DirectoryMode(t *testing.T) {
	setupAESCLITest(t)
	secrets := t.TempDir()
	pw := writeSecretFile(t, secrets, "pw.txt", aesCLITestPassword)
	keyFile := writeSecretFile(t, secrets, "key.hex", hex.EncodeToString(aesCLITestKey))

	dir := t.TempDir()
	files := map[string]string{"a.toml": "alpha", "b.toml": "beta"}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}

	require.NoError(t, runAESCLI(t, nil, "encrypt", "aes", "-i", dir, "--password-file", pw))
	var cts [][]byte
	for name, content := range files {
		ct, err := os.ReadFile(filepath.Join(dir, name+".enc"))
		require.NoError(t, err)
		_, err = gcrypto.AEADDecrypt([]byte(aesCLITestPassword), ct, nil)
		require.Error(t, err)
		got, err := gcrypto.DecryptByPassword([]byte(aesCLITestPassword), ct)
		require.NoError(t, err)
		require.Equal(t, content, string(got))
		cts = append(cts, ct)
	}
	const saltEnd, nonceEnd = 35, 47
	require.Equal(t, cts[0][:saltEnd], cts[1][:saltEnd], "key derived once per run")
	require.NotEqual(t, cts[0][saltEnd:nonceEnd], cts[1][saltEnd:nonceEnd], "fresh nonce per file")

	require.NoError(t, runAESCLI(t, nil, "encrypt", "aes", "-i", dir, "--key-file", keyFile))
	for name, content := range files {
		ct, err := os.ReadFile(filepath.Join(dir, name+".enc"))
		require.NoError(t, err)
		got, err := gcrypto.AEADDecrypt(aesCLITestKey, ct, nil)
		require.NoError(t, err)
		require.Equal(t, content, string(got))
	}
	_, err := os.Lstat(filepath.Join(dir, "a.toml.enc.enc"))
	require.ErrorIs(t, err, os.ErrNotExist, "existing outputs are not re-encrypted")

	err = runAESCLI(t, nil, "encrypt", "aes", "-i", dir, "-o", filepath.Join(dir, "x"), "--key-file", keyFile)
	require.ErrorContains(t, err, "--output is not supported")
}
