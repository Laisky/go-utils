# Sentinel's Journal

## 2025-02-01 - Weak Password Hashing and Timing Attacks

**Vulnerability:** The password hashing implementation used an extremely low iteration count (1-10) and relied on non-constant-time comparisons for verification. Additionally, sensitive byte slices were modified in-place, potentially leading to side effects.
**Learning:** Legacy utility functions often prioritize simplicity or speed over security. In custom cryptographic implementations, it's easy to overlook industry standards like minimum work factors or constant-time operations.
**Prevention:** Always use `crypto/subtle.ConstantTimeCompare` for sensitive comparisons. For password hashing, follow established standards (e.g., 10,000+ iterations for SHA-based loops, though Argon2/bcrypt are preferred). Ensure input slices are copied before modification.

## 2026-01-29 - [Insecure Cryptographic Practices]

**Vulnerability:** Use of `bytes.Equal` for sensitive comparisons and extremely low iteration count (1-10) for password hashing.
**Learning:** Even utility libraries can have custom crypto implementations that overlook standard security practices like constant-time comparison and adequate work factors.
**Prevention:** Always use `subtle.ConstantTimeCompare` or `hmac.Equal` for sensitive data. Follow OWASP recommendations for password hashing iterations (minimum 10,000 in this context).
