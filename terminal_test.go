package utils

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// reset os.Stdin to mock user input
			fp, err := NewTmpFile(bytes.NewReader([]byte(tt.args.input)))
			require.NoError(t, err)
			defer fp.Close()
			os.Stdin = fp

			if ok, err := InputYes(tt.args.question); (err != nil) != tt.wantErr {
				t.Errorf("InputYes() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				require.True(t, ok == tt.ok)
			}
		})
	}
}
