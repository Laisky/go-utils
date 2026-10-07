//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package fileguard

import (
	"os"
	"syscall"
)

// readFlags prevents a FIFO substituted after inspection from blocking open.
func readFlags() (int, error) { return os.O_RDONLY | syscall.O_NONBLOCK, nil }
