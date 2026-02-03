## 2026-02-03 - [Inefficient Date Truncation]
**Learning:** Using `time.Parse(layout, t.Format(layout))` to truncate a time to a date is extremely inefficient in Go (~10x slower than `Truncate`).
**Action:** Always use `t.Truncate(24 * time.Hour)` or `time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)` for date truncation.

## 2026-02-03 - [RWMutex vs Atomic for Simple Values]
**Learning:** Using `sync.RWMutex` for protecting a single `int64` or `time.Duration` field that is frequently read is significantly slower than using `atomic` operations.
**Action:** Use `atomic.Int64` or `atomic.LoadInt64` for simple numeric fields in hot paths.
