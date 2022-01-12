package utils

import (
	"testing"
	"time"

	"github.com/Laisky/go-utils/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestColor(t *testing.T) {
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

func TestGormLogger_Print(t *testing.T) {
	t.Run("gorm v1", func(t *testing.T) {
		logger := new(mocks.LoggerItf)
		logger.On("Info", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		logger.On("Debug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		logger.On("Error", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		for _, msg := range []interface{}{
			"drop",
			"delete",
			"insert",
			"update",
			"select",
			"error",
			[]byte("drop"),
			123,
		} {
			mockFomatter := func(...interface{}) []interface{} {
				return []interface{}{
					"",
					"",
					"",
					msg,
				}
			}
			gl := NewGormLogger(mockFomatter, logger)
			gl.Print(
				"type",
				"caller",
				time.Second,
				"sql",
				"args",
				"affected",
				"extras",
			)
		}

		require.Equal(t, len(logger.Calls), 8)
	})

	t.Run("short", func(t *testing.T) {
		mockFomatter := func(...interface{}) []interface{} {
			return []interface{}{
				"yo",
			}
		}
		gl := NewGormLogger(mockFomatter, Logger)
		gl.Print(
			"type",
			"caller",
			time.Second,
			"sql",
			"args",
			"affected",
			"extras",
		)
	})
}
