//go:build linux

package utils

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// zombieChildren returns the PIDs of zombie children of the current process
// whose command name is comm, read from /proc.
func zombieChildren(t *testing.T, comm string) []int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	require.NoError(t, err)

	self := os.Getpid()
	var zombies []int
	for _, entry := range entries {
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if readErr != nil {
			continue
		}
		// Format: pid (comm) state ppid ...; comm may contain spaces.
		stat := string(raw)
		open, closeIdx := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
		if open < 0 || closeIdx < open {
			continue
		}
		fields := strings.Fields(stat[closeIdx+1:])
		if len(fields) < 2 || stat[open+1:closeIdx] != comm {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		if fields[0] == "Z" && ppid == self {
			zombies = append(zombies, pid)
		}
	}
	return zombies
}

// TestOpenURLInDefaultBrowserReapsLauncher verifies that the launcher process
// started by OpenURLInDefaultBrowser is waited for after it exits, so repeated
// calls do not accumulate zombie processes in long-running programs.
func TestOpenURLInDefaultBrowserReapsLauncher(t *testing.T) {
	binDir := t.TempDir()
	launcher := filepath.Join(binDir, "xdg-open")
	require.NoError(t, os.WriteFile(launcher, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for range 3 {
		require.NoError(t, OpenURLInDefaultBrowser(ctx, "https://example.com/reap"))
	}

	require.Eventually(t, func() bool {
		return len(zombieChildren(t, "xdg-open")) == 0
	}, 5*time.Second, 20*time.Millisecond, "launcher processes were never reaped")
}
