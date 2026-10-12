package local

import (
	"time"

	"github.com/Laisky/errors/v2"
)

// ErrBudgetExceeded reports an exhausted local read or search resource budget.
var ErrBudgetExceeded = errors.New("local storage resource budget exceeded")

// Limits controls per-operation resource use. Zero fields select defaults;
// negative fields are rejected. Configuration is trusted application policy,
// not a per-request override supplied by an untrusted caller.
type Limits struct {
	MaxReadBytes         int64
	MaxSearchFiles       int
	MaxSearchEntries     int
	MaxSearchBytes       int64
	MaxSearchResultBytes int64
	SearchTimeout        time.Duration
}

// DefaultLimits returns bounded defaults: 8 MiB per Read, 1024 searched files,
// 8192 discovered entries, 64 MiB scanned, 8 MiB retained, and five seconds.
func DefaultLimits() Limits {
	return Limits{MaxReadBytes: 8 << 20, MaxSearchFiles: 1024, MaxSearchEntries: 8192, MaxSearchBytes: 64 << 20, MaxSearchResultBytes: 8 << 20, SearchTimeout: 5 * time.Second}
}

// normalizeLimits fills zero fields and rejects negative or unaddressable limits.
func normalizeLimits(limits Limits) (Limits, error) {
	if limits.MaxReadBytes < 0 || limits.MaxSearchFiles < 0 || limits.MaxSearchEntries < 0 || limits.MaxSearchBytes < 0 || limits.MaxSearchResultBytes < 0 || limits.SearchTimeout < 0 {
		return Limits{}, errors.New("local storage limits cannot be negative")
	}
	defaults := DefaultLimits()
	if limits.MaxReadBytes == 0 {
		limits.MaxReadBytes = defaults.MaxReadBytes
	}
	if limits.MaxSearchFiles == 0 {
		limits.MaxSearchFiles = defaults.MaxSearchFiles
	}
	if limits.MaxSearchEntries == 0 {
		limits.MaxSearchEntries = defaults.MaxSearchEntries
	}
	if limits.MaxSearchBytes == 0 {
		limits.MaxSearchBytes = defaults.MaxSearchBytes
	}
	if limits.MaxSearchResultBytes == 0 {
		limits.MaxSearchResultBytes = defaults.MaxSearchResultBytes
	}
	if limits.SearchTimeout == 0 {
		limits.SearchTimeout = defaults.SearchTimeout
	}
	if limits.MaxReadBytes > int64(int(^uint(0)>>1)) {
		return Limits{}, errors.New("read limit exceeds addressable memory")
	}
	return limits, nil
}
