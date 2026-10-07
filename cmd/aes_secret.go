package cmd

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Laisky/go-utils/v6/log"
)

const (
	// maxAESPasswordFileBytes bounds a password file or stdin password.
	maxAESPasswordFileBytes = 4096
	// maxAESKeyFileBytes bounds a hex key file or stdin key.
	maxAESKeyFileBytes = 1024
	// aesSecretFromStdin selects standard input as the secret source.
	aesSecretFromStdin = "-"
)

var (
	// aesIsTerminal reports whether fd is a terminal; tests replace it.
	aesIsTerminal = term.IsTerminal
	// aesReadPassword reads a line from the terminal fd without echo; tests replace it.
	aesReadPassword = term.ReadPassword
)

// aesCLISecret is the secret selected for one AES CLI run: exactly one of
// password (password mode) or key (raw-key mode) is set.
type aesCLISecret struct {
	password []byte
	key      []byte
}

// wipe zeroes the secret buffers on a best-effort basis.
func (s *aesCLISecret) wipe() {
	clear(s.password)
	clear(s.key)
}

// mode returns "raw-key" or "password" for logging; it never exposes the secret.
func (s *aesCLISecret) mode() string {
	if s.key != nil {
		return "raw-key"
	}
	return "password"
}

// readAESCLISecret selects the secret source: --key-file (raw-key mode),
// --password-file (password mode), or else an interactive no-echo prompt when
// stdin is a terminal (confirmed twice when confirm is true). Without any
// source it fails closed. It returns the secret or an error that never contains
// secret material.
func readAESCLISecret(cmd *cobra.Command, passwordFile, keyFile string, confirm bool) (*aesCLISecret, error) {
	switch {
	case passwordFile != "" && keyFile != "":
		return nil, errors.New("use only one of --password-file and --key-file")
	case keyFile != "":
		raw, err := readAESSecretSource(cmd, keyFile, maxAESKeyFileBytes)
		if err != nil {
			return nil, errors.Wrap(err, "read --key-file")
		}
		defer clear(raw)

		key, err := parseAESHexKey(raw)
		if err != nil {
			return nil, errors.Wrap(err, "parse --key-file")
		}
		return &aesCLISecret{key: key}, nil
	case passwordFile != "":
		raw, err := readAESSecretSource(cmd, passwordFile, maxAESPasswordFileBytes)
		if err != nil {
			return nil, errors.Wrap(err, "read --password-file")
		}

		password := bytes.TrimRight(raw, "\r\n")
		if len(password) == 0 {
			return nil, errors.New("--password-file holds an empty password")
		}
		return &aesCLISecret{password: password}, nil
	default:
		password, err := promptAESPassword(cmd, confirm)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return &aesCLISecret{password: password}, nil
	}
}

// readAESSecretSource reads at most limit bytes from path, or from the
// command's stdin when path is "-". A named file must be a regular file or a
// pipe and, on Unix, must not be accessible by group or others. It returns the
// raw bytes or an error that names the path but never the content.
func readAESSecretSource(cmd *cobra.Command, path string, limit int64) ([]byte, error) {
	if path == aesSecretFromStdin {
		return readAESSecretLimited(cmd.InOrStdin(), limit)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "open secret file `%s`", path)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Shared.Debug("close secret file", zap.String("path", path), zap.Error(err))
		}
	}()

	st, err := f.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "stat secret file `%s`", path)
	}
	if !st.Mode().IsRegular() && st.Mode()&os.ModeNamedPipe == 0 {
		return nil, errors.Errorf("secret file `%s` must be a regular file or a pipe", path)
	}
	if err := checkAESSecretFileMode(path, st.Mode()); err != nil {
		return nil, errors.WithStack(err)
	}

	return readAESSecretLimited(f, limit)
}

// checkAESSecretFileMode rejects secret files readable or writable by group or
// others on Unix-like systems; Windows ACLs are not inspected. It returns an
// error with a chmod hint, or nil.
func checkAESSecretFileMode(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if perm := mode.Perm(); perm&0o077 != 0 {
		return errors.Errorf("refusing secret file `%s`: it is accessible by group or others (mode %#o); "+
			"run `chmod 600 %s`", path, perm, path)
	}

	return nil
}

// readAESSecretLimited reads r up to limit bytes and fails when more is
// available. It returns the bytes read or an error.
func readAESSecretLimited(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		clear(raw)
		return nil, errors.Wrap(err, "read secret")
	}
	if int64(len(raw)) > limit {
		clear(raw)
		return nil, errors.Errorf("secret input exceeds %d bytes", limit)
	}

	return raw, nil
}

// parseAESHexKey strictly decodes a hex-encoded 16, 24 or 32-byte key after
// trimming surrounding whitespace. Its errors never echo the input, because
// encoding/hex errors quote the offending (secret) byte.
func parseAESHexKey(raw []byte) ([]byte, error) {
	src := bytes.TrimSpace(raw)
	for _, c := range src {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return nil, errors.New("key must be hex digits only (no prefix, separators or other characters)")
		}
	}

	switch len(src) {
	case 32, 48, 64:
	default:
		return nil, errors.Errorf("key must encode 16, 24 or 32 bytes (32, 48 or 64 hex digits), got %d digits",
			len(src))
	}

	key := make([]byte, hex.DecodedLen(len(src)))
	if _, err := hex.Decode(key, src); err != nil {
		clear(key)
		return nil, errors.New("key is not valid hex")
	}

	return key, nil
}

// promptAESPassword reads a password from the controlling terminal without
// echo, asking twice when confirm is true. When stdin is not a terminal it
// fails closed. It returns the password or an error.
func promptAESPassword(cmd *cobra.Command, confirm bool) ([]byte, error) {
	in, ok := cmd.InOrStdin().(*os.File)
	if !ok || !aesIsTerminal(int(in.Fd())) { //nolint:gosec // file descriptors fit in int.
		return nil, errors.New("no secret input: pass --password-file <path|-> or --key-file <path|->, " +
			"or run in an interactive terminal to be prompted; secrets are never accepted as arguments")
	}
	fd := int(in.Fd()) //nolint:gosec // file descriptors fit in int.
	w := cmd.ErrOrStderr()

	password, err := readAESPasswordLine(fd, w, "Enter password: ")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(password) == 0 {
		return nil, errors.New("empty password")
	}
	if !confirm {
		return password, nil
	}

	again, err := readAESPasswordLine(fd, w, "Confirm password: ")
	if err != nil {
		clear(password)
		return nil, errors.WithStack(err)
	}
	defer clear(again)
	if subtle.ConstantTimeCompare(password, again) != 1 {
		clear(password)
		return nil, errors.New("passwords do not match")
	}

	return password, nil
}

// readAESPasswordLine writes prompt to w and reads one no-echo line from fd.
// It returns the line without its terminator, or an error.
func readAESPasswordLine(fd int, w io.Writer, prompt string) ([]byte, error) {
	if _, err := fmt.Fprint(w, prompt); err != nil {
		return nil, errors.Wrap(err, "write password prompt")
	}
	password, err := aesReadPassword(fd)
	if _, werr := fmt.Fprintln(w); werr != nil {
		log.Shared.Debug("write prompt newline", zap.Error(werr))
	}
	if err != nil {
		return nil, errors.Wrap(err, "read password from terminal")
	}

	return password, nil
}
