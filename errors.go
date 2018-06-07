package utils

import (
	"fmt"
)

// HTTPInvalidStatusError return error about status code
func HTTPInvalidStatusError(statusCode int) error {
	return fmt.Errorf("got http invalid status code <%v>", statusCode)
}
