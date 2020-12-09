package utils

import (
	"io"
	"testing"

	"github.com/pkg/errors"
)

func TestErrorsIs(t *testing.T) {
	errEOF := errors.WithStack(io.EOF)
	if errEOF == io.EOF {
		t.Fatal()
	}

	if !errors.Is(errEOF, io.EOF) {
		t.Fatal()
	}
}
