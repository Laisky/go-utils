# Atomic keyed rate limiter

`KeyedRateLimiter` is an opt-in, process-local token bucket for a shared budget and a per-key budget. It starts both full and `Allow` spends one token from each atomically. A refusal spends neither. Rates are positive whole tokens per second; burst and rate fields each range from 1 through 1,000,000,000. Refills use integer token-nanoseconds, including fractional tokens, and clamp when full. No background worker is started and no shutdown is needed.

```go
limiter, err := utils.NewKeyedRateLimiter[string](utils.KeyedRateLimiterConfig{
    TotalPerSecond: 100,
    TotalBurst:     200,
    KeyPerSecond:   5,
    KeyBurst:       10,
    MaxKeys:        10000,
})
if err != nil {
    return err
}
if !limiter.Allow(authenticatedUserID) {
    // Reject or retry according to the application's policy.
}
```

The limiter keeps at most `MaxKeys` tracked entries. If a new key arrives at capacity, it can replace an existing key only after the existing key's budget has fully refilled. Otherwise the new key is refused without spending either budget. This choice preserves quotas but can deny legitimate new keys while the table is full. Fully refilled idle entries stay in memory until another key needs space; there is no timer or active cleanup. Expiry is tracked in a heap; creating or updating a key costs O(log MaxKeys), and each insertion removes at most one old key.

`MaxKeys` bounds the number of entries, not the byte size or external objects referenced by keys. Authenticate before admission when appropriate and validate and bound untrusted key representations. A dynamically incomparable key or a non-reflexive key such as NaN is refused. Calls are safe concurrently, and the limiter must not be copied after first use. The limiter provides no reservation, cancellation, refund, persistence, distributed coordination, or blocking wait.

`RateLimiter` is still appropriate for a single budget, `AllowN`, shared state managers, snapshots, and cloning. The keyed API adds an in-memory atomic global-plus-key decision and a hard key-count cap; it does not replace that API. Combining separate limiters without an exclusive lock and checking both before consuming can waste one budget on refusal. Wrapping `golang.org/x/time/rate` buckets with exclusive synchronization and a safe eviction policy is another viable design when its API and semantics are a better fit.
