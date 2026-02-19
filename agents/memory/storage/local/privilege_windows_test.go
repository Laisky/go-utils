//go:build windows

package local

// isRunningAsRoot reports whether the current test process runs with root privileges.
func isRunningAsRoot() bool {
	return false
}
