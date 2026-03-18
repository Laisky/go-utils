## 2026-03-17 - HS256 Secret Length Enforcement
**Vulnerability:** The JWT package defined the RFC 7518 minimum HS256 secret length but accepted shorter HMAC secrets, making weak signing keys easy to configure accidentally.
**Learning:** Security requirements that exist only as constants or comments are ineffective unless option setters enforce them at the API boundary.
**Prevention:** Keep cryptographic invariants validated in constructor and option paths, and add tests that reject undersized secrets as invalid configuration.