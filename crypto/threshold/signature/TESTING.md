# Threshold RSA test policy

Routine tests use two public, test-only safe primes to avoid repeating an
unbounded random safe-prime search under race instrumentation. The modulus is
at least 2048 bits. Both primes and both `(prime-1)/2` factors are checked for
primality before use. Real `tcrsa` share generation, random coefficients,
signature proofs, all 3-of-5 member combinations, and independent standard-library
verification remain exercised, including with `go test -short`.

The fixtures are not suitable for deployment: their factors are public. They
exist only in `_test.go`, and production `NewKeyShares` is unchanged.

The expensive production success path is **not silently removed**. Run it
explicitly before a cryptographic dependency or key-generation change:

```sh
GO_UTILS_RUN_THRESHOLD_KEYGEN=1 go test -race -count=1 -timeout=10m \
  -run '^TestNewKeySharesRandomIntegration$' ./crypto/threshold/signature
```

It uses fresh production randomness in a child process, with an eight-minute
wall-clock limit and guaranteed process reaping. Exceeding that limit is an
integration failure, not a pass or a signal to lower RSA key strength. Routine
integer/size validation still calls the public constructor on every suite run.

Historical timeout: issue #86, Actions run 37377416828 attempt 1, artifact
11372727882, contains a five-minute `TestVerifyBySHA256` timeout in safe-prime
generation. An unchanged retry passing does not resolve that random runtime.
