# Sentinel's Journal

## 2026-01-29 - [Insecure Cryptographic Practices]
**Vulnerability:** Use of `bytes.Equal` for sensitive comparisons and extremely low iteration count (1-10) for password hashing.
**Learning:** Even utility libraries can have custom crypto implementations that overlook standard security practices like constant-time comparison and adequate work factors.
**Prevention:** Always use `subtle.ConstantTimeCompare` or `hmac.Equal` for sensitive data. Follow OWASP recommendations for password hashing iterations (minimum 10,000 in this context).
