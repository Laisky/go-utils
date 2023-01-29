package utils

import "fmt"

// Wrap wrap error without stack
func Wrap(err error, msg string) error {
	return fmt.Errorf("%s: %w", msg, err)
}
