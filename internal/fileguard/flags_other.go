//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package fileguard

import "github.com/Laisky/errors/v2"

// readFlags fails closed where safe source-open behavior has not been implemented.
func readFlags() (int, error) {
	return 0, errors.New("safe source opening is unsupported on this platform")
}
