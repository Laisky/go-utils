# Sentinel's Journal

## 2025-02-01 - Weak Password Hashing and Timing Attacks

**Vulnerability:** The password hashing implementation used an extremely low iteration count (1-10) and relied on non-constant-time comparisons for verification. Additionally, sensitive byte slices were modified in-place, potentially leading to side effects.
**Learning:** Legacy utility functions often prioritize simplicity or speed over security. In custom cryptographic implementations, it's easy to overlook industry standards like minimum work factors or constant-time operations.
**Prevention:** Always use `crypto/subtle.ConstantTimeCompare` for sensitive comparisons. For password hashing, follow established standards (e.g., 10,000+ iterations for SHA-based loops, though Argon2/bcrypt are preferred). Ensure input slices are copied before modification.

## 2026-01-29 - [Insecure Cryptographic Practices]

**Vulnerability:** Use of `bytes.Equal` for sensitive comparisons and extremely low iteration count (1-10) for password hashing.
**Learning:** Even utility libraries can have custom crypto implementations that overlook standard security practices like constant-time comparison and adequate work factors.
**Prevention:** Always use `subtle.ConstantTimeCompare` or `hmac.Equal` for sensitive data. Follow OWASP recommendations for password hashing iterations (minimum 10,000 in this context).

## 2025-05-15 - Password Hashing DoS Protection

**Vulnerability:** Lack of upper limit on iteration count for password hashing allowed for CPU-exhaustion Denial of Service (DoS) attacks via specially crafted password hashes.
**Learning:** Even with an enforced delay for password verification, an attacker can still cause significant CPU load by specifying a very high iteration count in the hash string, which is processed before the delay completes or consumes CPU time that the delay doesn't account for.
**Prevention:** Always implement a strict upper limit on iteration counts for all iterative cryptographic operations (like PBKDF2, bcrypt, or custom hash loops) that are derived from user-controllable input.

## 2026-02-02 - Denial of Service via Uncontrolled Iteration Count

**Vulnerability:** The password verification logic allowed the iteration count to be parsed directly from the hash string without any upper bound. An attacker could provide a hash with a very large iteration count (e.g., millions), causing the server to consume excessive CPU resources.
**Learning:** While password hashing is intended to be slow, allowing the input to arbitrarily increase the work factor leads to resource exhaustion vulnerabilities.
**Prevention:** Always enforce a reasonable maximum iteration count (e.g., 1,000,000) when parsing cryptographic parameters from untrusted input.

## 2026-02-11 - RBAC Partial Match and Hierarchical Logic Vulnerability

**Vulnerability:** The RBAC permission containment check used a simple prefix match, allowing "root.sysadmin" to match "root.sys". This could lead to unintended access or revocation. Additionally, it lacked support for hierarchical matching and wildcards.
**Learning:** Core security logic like RBAC must be precisely implemented with boundary checks to prevent "leaking" permissions between similarly named keys.
**Prevention:** Always use delimiter-aware prefix checks (e.g., appending a dot to the prefix) and comprehensive test suites covering partial matches and wildcards.
