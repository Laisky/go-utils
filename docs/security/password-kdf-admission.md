# Shared password-ciphertext KDF admission (#117)

Password encryption constructors and ciphertext decryption now use the same
process-wide weighted Argon2 semaphore as password hashing. The existing aggregate
budget is 512 MiB in Argon2 memory units; ciphertext parameters are validated
before admission and retain their existing per-operation ceilings. This bounds
admitted Argon2 work, not total process RSS, queuing, CPU time, or application
request rates. Applications must still bound queues, deadlines and concurrency.

The wire format, default KDF parameters, and existing APIs are unchanged. Existing
calls now wait when the shared budget is occupied. New context-aware variants are
`NewPasswordEncryptorContext`, `EncryptByPasswordContext`, and
`DecryptByPasswordContext`. Cancellation interrupts waiting admission; Go's
Argon2 implementation cannot interrupt a derivation that has already started.

The reproduction occupied the complete logical semaphore without allocating a
large KDF buffer. Low-cost encryption and decryption incorrectly completed anyway
on baseline `042437b6015f5ba8f185ef91b05b2bd3a5a420f3`. Regression tests require no
KDF entry before release and no derivation on canceled admission. Run
`go test -race -run 'TestPasswordAdmission|TestPassword' ./crypto`.

No ciphertext migration is needed. Rollout should track waiting/canceled requests
and set context deadlines at untrusted entry points. A binary rollback preserves
ciphertext compatibility but reintroduces the admission gap.
