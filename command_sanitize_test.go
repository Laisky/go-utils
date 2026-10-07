package utils

import (
	"reflect"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestSanitizeCMDArgs verifies that SanitizeCMDArgs trims surrounding whitespace from each argument and keeps a bare
// "$", while rejecting "$(...)" command substitution and arguments that contain newline, carriage-return, or NUL
// control characters.
func TestSanitizeCMDArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expected    []string
		expectedErr error
	}{
		{
			name:        "0",
			args:        []string{"arg1", "arg2", "arg3"},
			expected:    []string{"arg1", "arg2", "arg3"},
			expectedErr: nil,
		},
		{
			name:        "1",
			args:        []string{"arg1", "arg2$", "arg3"},
			expected:    []string{"arg1", "arg2$", "arg3"},
			expectedErr: nil,
		},
		{
			name:        "2",
			args:        []string{"arg1", "arg2$(echo hello)", "arg3"},
			expected:    nil,
			expectedErr: errors.New("invalid command substitution in args"),
		},
		{
			name:        "3",
			args:        []string{"  arg1  ", "arg2  ", "  arg3"},
			expected:    []string{"arg1", "arg2", "arg3"},
			expectedErr: nil,
		},
		{
			name:        "reject newline",
			args:        []string{"arg1", "arg2\ninjected"},
			expected:    nil,
			expectedErr: errors.New("control characters in args"),
		},
		{
			name:        "reject null byte",
			args:        []string{"arg1\x00injected"},
			expected:    nil,
			expectedErr: errors.New("control characters in args"),
		},
		{
			name:        "reject carriage return",
			args:        []string{"arg1\rinjected"},
			expected:    nil,
			expectedErr: errors.New("control characters in args"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sanitizedArgs, err := SanitizeCMDArgs(test.args)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())
				return
			}

			isDeepEqual := reflect.DeepEqual(sanitizedArgs, test.expected)
			require.True(t, isDeepEqual, "sanitizedArgs: %v, expected: %v",
				sanitizedArgs, test.expected)
		})
	}
}
