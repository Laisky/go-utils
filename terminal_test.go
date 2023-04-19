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
		wantErr bool
	}{
		{"0", args{"test", "y\n"}, false},
		{"1", args{"test", "Y\n"}, false},
		{"2", args{"test", "n\n"}, true},
		{"3", args{"test", "N\n"}, true},
		{"4", args{"test", "N"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// reset os.Stdin to mock user input
			fp, err := NewTmpFile(bytes.NewReader([]byte(tt.args.input)))
			require.NoError(t, err)
			defer fp.Close()
			os.Stdin = fp

			if err := InputYes(tt.args.question); (err != nil) != tt.wantErr {
				t.Errorf("InputYes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
