//go:build !linux

package utils

import (
	"github.com/Laisky/errors/v2"
)

// InputPassword is not supported on non-Linux platforms
func InputPassword(hint string, validator func(string) error) (passwd string, err error) {
	return "", errors.New("InputPassword is not supported on this platform")
}
