package utils

import (
	"sync"
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestUUID1 verifies that UUID1 produces 10000 unique values when called from concurrent goroutines, and that
// sequentially generated values carry timestamps no earlier than that of the first value.
func TestUUID1(t *testing.T) {
	t.Run("goroutine", func(t *testing.T) {
		var (
			mu   sync.Mutex
			uids []string
		)

		var pool errgroup.Group
		for i := 0; i < 10000; i++ {
			pool.Go(func() error {
				uid := UUID1()

				mu.Lock()
				uids = append(uids, uid)
				mu.Unlock()
				return nil
			})
		}

		require.NoError(t, pool.Wait())
		require.Len(t, uids, 10000)

		st := mapset.NewSet(uids...)
		require.Equal(t, len(uids), st.Cardinality())
	})

	t.Run("monotonically time", func(t *testing.T) {
		var uids []string
		for i := 0; i < 10000; i++ {
			uid := UUID1()
			uids = append(uids, uid)
		}

		var lastTime time.Time
		for i := range uids {
			uid, err := uuid.Parse(uids[i])
			require.NoError(t, err)

			ct := time.Unix(uid.Time().UnixTime())
			if lastTime.IsZero() {
				lastTime = ct
				continue
			}

			require.GreaterOrEqual(t, ct, lastTime)
		}
	})
}

// TestUUID4 verifies that UUID4 returns a parseable UUID and produces 10000 unique values across 10 concurrent
// goroutines.
func TestUUID4(t *testing.T) {
	t.Parallel()
	val := UUID4()
	_, err := uuid.Parse(val)
	require.NoError(t, err)

	t.Run("unique", func(t *testing.T) {
		t.Parallel()

		var (
			mu      sync.Mutex
			pool    errgroup.Group
			uuidSet = map[string]struct{}{}
		)
		for i := 0; i < 10; i++ {
			pool.Go(func() error {
				for i := 0; i < 1000; i++ {
					uuid := UUID4()
					mu.Lock()
					uuidSet[uuid] = struct{}{}
					mu.Unlock()
				}

				return nil
			})
		}

		require.NoError(t, pool.Wait())
		require.Len(t, uuidSet, 10000)
	})
}

// TestUUID7 verifies that UUID7 produces 10000 unique values across 10 concurrent goroutines, that every value is
// accepted by ParseUUID7, and that a value generated one millisecond later sorts after the earlier one.
func TestUUID7(t *testing.T) {
	t.Parallel()
	var (
		mu      sync.Mutex
		pool    errgroup.Group
		uuidSet = map[string]struct{}{}
	)
	for i := 0; i < 10; i++ {
		pool.Go(func() error {
			for i := 0; i < 1000; i++ {
				uuid := UUID7()
				mu.Lock()
				uuidSet[uuid] = struct{}{}
				mu.Unlock()
			}

			return nil
		})
	}

	require.NoError(t, pool.Wait())
	require.Len(t, uuidSet, 10000)

	for val := range uuidSet {
		_, err := ParseUUID7(val)
		require.NoError(t, err)
	}

	t.Run("order", func(t *testing.T) {
		t.Parallel()

		v1 := UUID7()
		time.Sleep(time.Millisecond)
		v2 := UUID7()
		require.Greater(t, v2, v1)
	})
}

// TestUUID7Bytes verifies that UUID7Bytes returns distinct 16-byte values carrying UUID version 7 and the
// RFC 4122 variant bits.
func TestUUID7Bytes(t *testing.T) {
	t.Parallel()

	// Generate UUID7 bytes
	b1 := UUID7Bytes()
	b2 := UUID7Bytes()

	require.Len(t, b1, 16, "UUID7Bytes should return 16 bytes")
	require.Len(t, b2, 16, "UUID7Bytes should return 16 bytes")
	require.NotEqual(t, b1, b2, "UUID7Bytes should generate unique values")

	// Check version (UUIDv7)
	version := (b1[6] >> 4) & 0x0F
	require.Equal(t, byte(0x07), version, "UUID7Bytes should have version 7")

	// Check variant (RFC4122)
	variant := (b1[8] >> 6) & 0x03
	require.Equal(t, byte(0x02), variant, "UUID7Bytes should have RFC4122 variant")
}
