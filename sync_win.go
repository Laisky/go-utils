//go:build windows
// +build windows

package utils

import (
	"github.com/Laisky/errors/v2"
)

// NewFlock new file lock
func NewFlock(lockFilePath string) (FLock, error) {
	return nil, errors.New("not implemented")
}
