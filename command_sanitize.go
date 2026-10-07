package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"
)

var reInvalidCMDChars = regexp.MustCompile(`[;&|]`)

// containsControlChars reports whether s contains any ASCII control characters
// (e.g. null bytes, newlines, carriage returns) that could cause argument
// truncation or injection when passed to external programs.
func containsControlChars(s string) bool {
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

// SanitizeCMDArgs sanitizes the given command arguments.
func SanitizeCMDArgs(args []string) (sanitizedArgs []string, err error) {
	for i, arg := range args {
		// Reject control characters (null bytes, newlines, etc.) that could
		// cause argument truncation or injection in external programs.
		if containsControlChars(arg) {
			return nil, errors.New("control characters in args")
		}

		// Check for invalid characters using a regular expression
		if reInvalidCMDChars.MatchString(arg) {
			return nil, errors.New("invalid characters in args")
		}

		// Check for command substitution
		if strings.Contains(arg, "$(") || strings.Contains(arg, "`") {
			return nil, errors.New("invalid command substitution in args")
		}

		// Trim leading and trailing whitespace
		args[i] = strings.TrimSpace(arg)
	}

	return args, nil
}

// resolveExecutablePath resolves an executable name or validates an explicit executable path.
func resolveExecutablePath(app string) (string, error) {
	app = strings.TrimSpace(app)
	if app == "" {
		return "", errors.New("app cannot be empty")
	}
	if containsControlChars(app) {
		return "", errors.New("control characters in app")
	}
	if reInvalidCMDChars.MatchString(app) || strings.Contains(app, "`") || strings.Contains(app, "$(") {
		return "", errors.New("invalid characters in app")
	}

	if strings.Contains(app, string(os.PathSeparator)) {
		cleaned := filepath.Clean(app)
		if cleaned == "." || cleaned == string(os.PathSeparator) {
			return "", errors.New("invalid app path")
		}

		return cleaned, nil
	}

	resolved, err := exec.LookPath(app)
	if err != nil {
		return "", errors.Wrap(err, "look path")
	}

	return resolved, nil
}
