package utils

import (
	"testing"
)

// TestColor verifies that Color wraps a string in the bold ANSI escape sequence for the given color code and
// appends the reset sequence, using the red foreground color as the table case.
func TestColor(t *testing.T) {
	t.Parallel()

	type args struct {
		color int
		s     string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"fg-red", args{ANSIColorFgRed, "yo"}, "\033[1;31myo\033[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Color(tt.args.color, tt.args.s); got != tt.want {
				t.Errorf("Color() = %v, want %v", got, tt.want)
			}
		})
	}
}
