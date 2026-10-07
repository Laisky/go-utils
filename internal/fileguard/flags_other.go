//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package fileguard

import (
	"os"

	"github.com/Laisky/errors/v2"
)

// readFlags fails closed where safe source-open behavior has not been implemented.
func readFlags() (int, error) {
	return 0, errors.New("safe source opening is unsupported on this platform")
}

// appendFlags returns no extra flags; OpenAppend relies on Lstat inspection plus
// post-open identity and type verification where O_NOFOLLOW is unavailable.
func appendFlags() int { return 0 }

// hasExtraLinks reports false because no portable link count is available here.
// It takes descriptor metadata and always returns false.
func hasExtraLinks(os.FileInfo) bool { return false }
