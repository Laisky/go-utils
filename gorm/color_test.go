package gorm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v3/log"
	"github.com/Laisky/go-utils/v3/mocks"
)

func TestGormLogger_Print(t *testing.T) {
	t.Run("gorm v1", func(t *testing.T) {
		logger := new(mocks.LoggerItf)
		logger.On("Info", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		logger.On("Debug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		logger.On("Error", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		for _, msg := range []any{
			"drop",
			"delete",
			"insert",
			"update",
			"select",
			"error",
			[]byte("drop"),
			123,
		} {
			mockFomatter := func(...any) []any {
				return []any{
					"",
					"",
					"",
					msg,
				}
			}
			gl := NewLogger(mockFomatter, logger)
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
		mockFomatter := func(...any) []any {
			return []any{
				"yo",
			}
		}
		gl := NewLogger(mockFomatter, log.Shared)
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
