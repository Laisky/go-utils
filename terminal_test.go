package utils

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInputYes verifies InputYes by replacing os.Stdin with a temp file of mocked input: "y" or "Y" and empty input
// (a bare newline or EOF) return true, while "n" or "N" with or without a trailing newline return false, all
// without error.
func TestInputYes(t *testing.T) {
	type args struct {
		question string
		input    string
	}
	tests := []struct {
		name    string
		args    args
		ok      bool
		wantErr bool
	}{
		{"0", args{"test", "y\n"}, true, false},
		{"1", args{"test", "Y\n"}, true, false},
		{"2", args{"test", "n\n"}, false, false},
		{"3", args{"test", "N\n"}, false, false},
		{"4", args{"test", "N"}, false, false},
		{"5", args{"test", "\n"}, true, false},
		{"6", args{"test", ""}, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// reset os.Stdin to mock user input
			fp, err := NewTmpFile(bytes.NewReader([]byte(tt.args.input)))
			require.NoError(t, err)
			defer fp.Close()
			os.Stdin = fp

			if ok, err := InputYes(tt.args.question); (err != nil) != tt.wantErr {
				t.Errorf("[%s]InputYes() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			} else {
				require.True(t, ok == tt.ok)
			}
		})
	}
}
