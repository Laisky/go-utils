package utils

import (
	"io"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

func TestErrorsIs(t *testing.T) {
	t.Parallel()

	rawErr := io.EOF
	wrappedErr := Wrap(rawErr, "wrap")

	ok := errors.Is(wrappedErr, rawErr)
	require.True(t, ok)

	t.Logf("%+v", wrappedErr)
	// t.Error()
}
