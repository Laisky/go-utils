## 2026-02-05 - [Optimize ClockT performance]
**Learning:** Using 't.Truncate(24 * time.Hour)' for date truncation in UTC is significantly faster (~9x) than 'time.Parse(layout, t.Format(layout))'. Also, using 'atomic.Int64' (available in Go 1.19+) or the new 'atomic.Int64' type in Go 1.25 is more efficient than 'sync.RWMutex' for simple numeric fields in high-concurrency paths.
**Action:** Always prefer 'Truncate' for date truncation and 'atomic' for simple shared state in performance-critical code.
