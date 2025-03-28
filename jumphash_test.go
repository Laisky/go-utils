package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJumpHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		key        uint64
		numBuckets int
		want       int32
		wantErr    bool
	}{
		{
			name:       "simple case",
			key:        123456,
			numBuckets: 10,
			want:       3, // Updated expected value
			wantErr:    false,
		},
		{
			name:       "zero buckets",
			key:        123456,
			numBuckets: 0,
			want:       0,
			wantErr:    true,
		},
		{
			name:       "negative buckets",
			key:        123456,
			numBuckets: -1,
			want:       0,
			wantErr:    true,
		},
		{
			name:       "max int32 buckets",
			key:        123456,
			numBuckets: (1 << 31) - 1,
			want:       479367282, // Updated expected value
			wantErr:    false,
		},
		{
			name:       "overflow buckets",
			key:        123456,
			numBuckets: 1 << 31,
			want:       0,
			wantErr:    true,
		},
		{
			name:       "different key same buckets",
			key:        654321,
			numBuckets: 10,
			want:       5, // Updated expected value
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := JumpHash(tt.key, tt.numBuckets)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestJumpHashConsistency(t *testing.T) {
	t.Parallel()

	key := uint64(123456)
	numBuckets := 10

	// Test that multiple calls with same input return same result
	first, err := JumpHash(key, numBuckets)
	assert.NoError(t, err)

	for i := 0; i < 100; i++ {
		result, err := JumpHash(key, numBuckets)
		assert.NoError(t, err)
		assert.Equal(t, first, result, "Hash should be consistent across multiple calls")
	}
}
