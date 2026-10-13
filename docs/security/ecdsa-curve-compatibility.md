# Standard-curve wrapper compatibility

ECDSA verification accepts wrappers whose mathematical parameters exactly match
P-224, P-256, P-384 or P-521. Object identity and a curve name are not the contract.
The verifier validates every parameter and uses a detached public key with the
standard-library curve implementation. It never invokes wrapper point arithmetic
and does not modify the caller's key. Different custom curves remain unsupported.

Invalid points, infinity, out-of-field coordinates, zero or out-of-range signature
components and malformed encodings still fail closed. Broken parameter accessors
are rejected. Regression tests use all three verification APIs, an independent
standard-library control, all supported curves, and wrappers with hostile arithmetic.

No key, signature or storage format changes; existing users need no migration.
