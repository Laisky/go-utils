package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateFileHash(t *testing.T) {
	t.Parallel()

	fp, err := os.CreateTemp("", "go-utils-*")
	require.NoError(t, err)
	defer os.Remove(fp.Name())
	defer fp.Close()

	content := []byte("jijf32ijr923e890dsfuodsafjlj;f9o2ur9re")
	_, err = fp.Write(content)
	require.NoError(t, err)

	err = ValidateFileHash(fp.Name(), "sha256:123")
	require.Error(t, err)

	err = ValidateFileHash(fp.Name(), "md5:123")
	require.Error(t, err)

	err = ValidateFileHash(fp.Name(), "sha254:123")
	require.Error(t, err)

	err = ValidateFileHash(fp.Name(), "")
	require.Error(t, err)

	err = ValidateFileHash(
		fp.Name(),
		"sha256:aea7e26c0e0b12ad210a8a0e45c379d0325b567afdd4b357158059b0ef03ae67",
	)
	require.NoError(t, err)

	err = ValidateFileHash(
		fp.Name(),
		"md5:794e37eea6b3df6e6eba69eb02f9b8c7",
	)
	require.NoError(t, err)
}
