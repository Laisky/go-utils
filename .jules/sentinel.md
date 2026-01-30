# Sentinel's Journal

## 2026-01-30 - Insecure Custom Password Hashing and Timing Attacks
**Vulnerability:** Extremely low iteration count (1-10) in custom password hashing and non-constant-time comparison using `bytes.Equal`.
**Learning:** The project used a custom iterated hash construction that relied on a very small random number of iterations. This made passwords vulnerable to brute-force. Additionally, sensitive byte comparisons (password hashes, signatures) used standard `bytes.Equal` which is susceptible to timing attacks.
**Prevention:** Use a minimum iteration count of 10,000 for iterated hashes and always use `crypto/subtle.ConstantTimeCompare` or `crypto/hmac.Equal` for sensitive comparisons. Prefer established libraries like `bcrypt` or `argon2` for password hashing when possible.
