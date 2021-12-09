package utils

import "github.com/pkg/errors"

// HTTPInvalidStatusError return error about status code
func HTTPInvalidStatusError(statusCode int) error {
	return errors.Errorf("got http invalid status code `%d`", statusCode)
}
