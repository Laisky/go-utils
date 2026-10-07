//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package fileguard

import (
	"os"
	"syscall"
)

// readFlags prevents a FIFO substituted after inspection from blocking open.
func readFlags() (int, error) { return os.O_RDONLY | syscall.O_NONBLOCK, nil }

// appendFlags refuses a final symbolic link and prevents a FIFO from blocking open.
// It returns the extra flags OpenAppend adds to every open.
func appendFlags() int { return syscall.O_NOFOLLOW | syscall.O_NONBLOCK }

// hasExtraLinks reports whether info describes an inode with more than one hard link.
// It takes descriptor metadata and returns false when the link count is unavailable.
func hasExtraLinks(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}
