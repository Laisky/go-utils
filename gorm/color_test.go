package gorm

import (
	"testing"
	"time"

	"github.com/Laisky/go-utils/v2/log"
	"github.com/Laisky/go-utils/v2/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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
		mockFomatter := func(...interface{}) []interface{} {
			return []interface{}{
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
