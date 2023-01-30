package utils

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorsIs(t *testing.T) {
	rawErr := io.EOF
	wrappedErr := Wrap(rawErr, "wrap")

	ok := errors.Is(wrappedErr, rawErr)
	require.True(t, ok)

	t.Logf("%+v", wrappedErr)
	// t.Error()
}
