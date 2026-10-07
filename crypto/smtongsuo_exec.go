package crypto

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"

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
	tongsuoCipherSM4CBC = "-sm4-cbc"
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
// environment. It returns the combined output or an error.
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
// returns the combined output or an error.
func (t *Tongsuo) runCMDWithEnv(ctx context.Context, args []string, stdin []byte, extraEnv []string) (
	output []byte, err error) {
	if args, err = gutils.SanitizeCMDArgs(args); err != nil {
		return nil, errors.Wrap(err, "sanitize cmd args")
	}

	//nolint: gosec
	// G204: Subprocess launched with a potential tainted input or cmd arguments
	cmd := exec.CommandContext(ctx, t.exePath, args...)
	cmd.Env = t.subprocessEnv(extraEnv)
	if len(stdin) != 0 {
		var stdinBuf bytes.Buffer
		stdinBuf.Write(stdin)
		cmd.Stdin = &stdinBuf
	}

	if output, err = cmd.CombinedOutput(); err != nil {
		return nil, errors.Wrapf(err, "run cmd failed, got %s", output)
	}

	return output, nil
}
