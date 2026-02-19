//go:build !windows

package local

import "os"

// isRunningAsRoot reports whether the current test process runs with root privileges.
func isRunningAsRoot() bool {
	return os.Geteuid() == 0
}
