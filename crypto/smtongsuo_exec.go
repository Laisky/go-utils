package crypto

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"

	gutils "github.com/Laisky/go-utils/v6"
)

// Tongsuo subcommand and flag names shared by the Tongsuo wrappers.
const (
	tongsuoCmdX509      = "x509"
	tongsuoCmdReq       = "req"
	tongsuoFlagInform   = "-inform"
	tongsuoFlagOutform  = "-outform"
	tongsuoFlagIn       = "-in"
	tongsuoFlagOut      = "-out"
	tongsuoFlagText     = "-text"
	tongsuoFlagConfig   = "-config"
	tongsuoFormatDER    = "DER"
	tongsuoStdinPath    = "/dev/stdin"
	tongsuoDigestSM3    = "-sm3"
	tongsuoDigestSHA256 = "-sha256"
	tongsuoCmdDgst      = "dgst"
)

// tongsuoInheritedEnvAllowlist names the only parent environment variables
// passed to tongsuo subprocesses by default: dynamic loader search paths that
// a relocated Tongsuo installation needs in order to start, and SYSTEMROOT,
// which Windows needs for its cryptographic runtime. Everything else,
// including OPENSSL_CONF and application secrets, is withheld.
var tongsuoInheritedEnvAllowlist = []string{
	"LD_LIBRARY_PATH",
	"DYLD_LIBRARY_PATH",
	"DYLD_FALLBACK_LIBRARY_PATH",
	"LIBPATH",
	"SYSTEMROOT",
}

// TongsuoOption configures a Tongsuo wrapper created by NewTongsuo.
type TongsuoOption func(*Tongsuo) error

// WithTongsuoInheritedEnv allows the named parent environment variables to be
// passed to tongsuo subprocesses in addition to the built-in loader allowlist.
// Use it only for deployment settings the binary genuinely needs, such as
// OPENSSL_CONF or OPENSSL_MODULES; never for application secrets. Each name
// must be non-empty and must not contain '=' or NUL. It returns the option.
func WithTongsuoInheritedEnv(names ...string) TongsuoOption {
	return func(t *Tongsuo) error {
		for _, name := range names {
			if name == "" || strings.ContainsAny(name, "=\x00") {
				return errors.Errorf("invalid environment variable name %q", name)
			}
			t.inheritedEnv = append(t.inheritedEnv, name)
		}

		return nil
	}
}

// envNameAllowed reports whether name is in allowed, comparing
// case-insensitively on Windows where environment names are case-insensitive.
func envNameAllowed(name string, allowed []string) bool {
	for _, a := range allowed {
		if name == a || (runtime.GOOS == "windows" && strings.EqualFold(name, a)) {
			return true
		}
	}

	return false
}

// subprocessEnv builds the complete, explicit environment for one tongsuo
// subprocess: allowlisted parent variables followed by extraEnv. It always
// returns a non-nil slice, so the parent environment is never inherited
// implicitly.
func (t *Tongsuo) subprocessEnv(extraEnv []string) []string {
	allowed := append([]string{}, tongsuoInheritedEnvAllowlist...)
	if runtime.GOOS == "windows" {
		allowed = append(allowed, "PATH") // DLL search path
	}
	allowed = append(allowed, t.inheritedEnv...)

	env := make([]string, 0, len(allowed)+len(extraEnv))
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if ok && envNameAllowed(name, allowed) {
			env = append(env, kv)
		}
	}

	return append(env, extraEnv...)
}

// runCMD runs a tongsuo command with stdin and the minimal subprocess
// environment. It returns the command's stdout only, or an error.
func (t *Tongsuo) runCMD(ctx context.Context, args []string, stdin []byte) (
	output []byte, err error) {
	return t.runCMDWithEnv(ctx, args, stdin, nil)
}

// runCMDWithEnv runs a tongsuo command with optional extra environment variables.
//
// The subprocess never inherits the parent environment: it receives only the
// allowlisted loader variables (see subprocessEnv) plus extraEnv. Use extraEnv
// to pass sensitive values (keys, passwords) via process environment instead
// of command-line arguments, which would be visible in the process list. It
// returns the command's stdout only; stderr diagnostics are never mixed into
// the result. On failure it returns an error carrying a bounded, sanitized
// stderr excerpt and never any stdout, which may hold key material.
func (t *Tongsuo) runCMDWithEnv(ctx context.Context, args []string, stdin []byte, extraEnv []string) (
	output []byte, err error) {
	output, _, err = t.runCMDOutputs(ctx, args, stdin, extraEnv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return output, nil
}

// runCMDOutputs runs a tongsuo command and captures stdout and stderr
// separately. Stdout is returned in full because it carries the command's
// data; stderr is kept up to maxTongsuoStderrCapture bytes. The parameters are
// as for runCMDWithEnv. On failure it returns nil outputs and an error that
// includes only sanitizeTongsuoStderr of the captured stderr.
func (t *Tongsuo) runCMDOutputs(ctx context.Context, args []string, stdin []byte, extraEnv []string) (
	stdout, stderr []byte, err error) {
	if args, err = gutils.SanitizeCMDArgs(args); err != nil {
		return nil, nil, errors.Wrap(err, "sanitize cmd args")
	}

	//nolint: gosec
	// G204: Subprocess launched with a potential tainted input or cmd arguments
	cmd := exec.CommandContext(ctx, t.exePath, args...)
	cmd.Env = t.subprocessEnv(extraEnv)
	if len(stdin) != 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdoutBuf bytes.Buffer
	stderrBuf := &cappedBuffer{limit: maxTongsuoStderrCapture}
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = stderrBuf

	if err = cmd.Run(); err != nil {
		subcommand := ""
		if len(args) != 0 {
			subcommand = args[0]
		}
		return nil, nil, errors.Wrapf(err, "run tongsuo %q failed, stderr: %s",
			subcommand, sanitizeTongsuoStderr(stderrBuf.buf.Bytes(), stderrBuf.truncated))
	}

	return stdoutBuf.Bytes(), stderrBuf.buf.Bytes(), nil
}

const (
	// maxTongsuoStderrCapture bounds the stderr bytes kept from one tongsuo run,
	// so a chatty or hostile binary cannot grow memory without limit.
	maxTongsuoStderrCapture = 64 << 10
	// maxTongsuoStderrExcerpt bounds the stderr excerpt embedded in an error.
	maxTongsuoStderrExcerpt = 512
	// tongsuoPEMBeginMarker starts every PEM block; nothing from it onward is
	// ever copied from stderr into an error, because it may be key material.
	tongsuoPEMBeginMarker = "-----BEGIN"
)

// cappedBuffer is an io.Writer that keeps at most limit bytes and discards the
// rest, recording in truncated that output was dropped. Write never fails, so
// the subprocess is never blocked or killed by the cap.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

// Write appends as much of p as fits under the limit and drops the rest. It
// always reports len(p) bytes written and a nil error.
func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); room < len(p) {
		c.truncated = true
		p = p[:max(room, 0)]
	}
	c.buf.Write(p)

	return len(p), nil
}

// sanitizeTongsuoStderr turns captured stderr into a short, single-line excerpt
// that is safe to embed in an error. Everything from the first PEM BEGIN marker
// onward is replaced by "[PEM redacted]", line breaks become " | ", other
// control characters and invalid UTF-8 become "?", and the result is cut to
// maxTongsuoStderrExcerpt bytes. truncated reports that the capture itself was
// cut. It returns "<empty>" when nothing remains.
func sanitizeTongsuoStderr(stderr []byte, truncated bool) string {
	text := string(stderr)
	redacted := false
	if i := strings.Index(text, tongsuoPEMBeginMarker); i >= 0 {
		text, redacted = text[:i], true
	}

	var b strings.Builder
	for _, r := range strings.ToValidUTF8(strings.TrimSpace(text), "?") {
		switch {
		case r == '\n':
			b.WriteString(" | ")
		case r == '\t' || r == ' ':
			b.WriteByte(' ')
		case unicode.IsControl(r) || r == utf8.RuneError:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}

	excerpt := b.String()
	if len(excerpt) > maxTongsuoStderrExcerpt {
		excerpt = strings.ToValidUTF8(excerpt[:maxTongsuoStderrExcerpt], "") + " ...(truncated)"
		truncated = false
	}
	if truncated {
		excerpt += " ...(truncated)"
	}
	if redacted {
		excerpt += " [PEM redacted]"
	}
	if strings.TrimSpace(excerpt) == "" {
		return "<empty>"
	}

	return excerpt
}
