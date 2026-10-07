# Password record formats and migration

`PasswordHash` now stores passwords with **Argon2id** (RFC 9106), a memory-hard
password KDF. Earlier releases stored an iterated general-purpose digest
(SHA-256/SHA-512 applied 10,000 to 1,000,000 times). Such records are no
longer created, but they still verify through a separate legacy branch. This
change addresses issue #93 (CodeQL `go/weak-sensitive-data-hashing`).

## New records

```
$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
```

- Standard PHC string. `<salt>` and `<hash>` use standard base64 **without
  padding**.
- Defaults follow the second recommended option in RFC 9106 section 4:
  `m=65536` KiB (64 MiB), `t=3` passes, `p=4` lanes, a 16-byte random salt and a
  32-byte tag.
- The `hasher` argument of `PasswordHash(password, hasher)` is kept only for API
  compatibility. It must still be `sha256` or `sha512`, and other values return
  the same error as before. Apart from that check it is ignored. No
  general-purpose digest is applied to the password.

## Verification (`VerifyHashedPassword`)

The record format is chosen strictly from the prefix. A failure in one branch
never retries in another branch.

| Record | Branch |
| --- | --- |
| starts with `$argon2id$` | Argon2id, strict parser |
| any other record containing `$` (`$argon2i$`, `$argon2d$`, `$scrypt$`, bcrypt `$2a$`, ...) | rejected |
| `<hasher>.<n>.<hexsalt>.<hexhash>` | legacy iterated digest, same behavior as before |
| anything else | rejected by the legacy parser |

The Argon2id parser checks every field before it starts any expensive work:

- version must be exactly `v=19`;
- parameters must be exactly `m=<n>,t=<n>,p=<n>`, in that order, as canonical
  positive decimals (no sign, no leading zero, no `keyid`/`data` fields);
- `p` must be in [1, 16], `t` in [1, 10], and `m` in [8*p, 262144] KiB (256 MiB);
- salt and tag must each be 16-64 bytes, in canonical unpadded base64. Padding,
  line breaks, the URL-safe alphabet and non-zero trailing bits are rejected;
- the record must have no extra fields and must not exceed
  `MaxHashedPasswordLength`.

The derived tag is compared with `subtle.ConstantTimeCompare`. All Argon2id
work in this package goes through a weighted semaphore with a 512 MiB memory
budget. This allows 8 concurrent default-cost operations, or 2 at the 256 MiB
cap, so concurrent verifications cannot multiply memory without limit. Extra
callers wait for budget. The `DefaultPasswordDelay` minimum duration and the
`MaxPasswordLength` limit are unchanged. Error messages never contain the
password or the stored record.

## Migration signal and rollout

`PasswordHashNeedsRehash(record) (bool, error)` performs no key derivation. It
returns:

- `true` for every well-formed legacy record;
- `true` for Argon2id records whose `m`, `t`, salt length or tag length is
  below the current defaults. Parallelism is not compared, because fewer lanes
  with the same `m` and `t` do not lower an attacker's cost;
- `false` for records that meet or exceed the defaults;
- an error for empty, oversized, malformed or unsupported records.

Recommended login flow:

1. `err := VerifyHashedPassword(pw, stored)`. Stop if it fails.
2. `needs, err := PasswordHashNeedsRehash(stored)`.
3. If `needs` is true, call `PasswordHash(pw, gutils.HashTypeSha256)` and replace
   the stored record. Only do this after a successful verification.

Rollout policy:

- Records written by this version cannot be verified by older go-utils
  releases. Upgrade every service that verifies passwords before any of them
  writes new records. During a rolling deploy, a node that has not been
  upgraded rejects records that were just created or re-hashed.
- Users who do not log in keep their legacy records. To retire the legacy
  branch, force a password reset for accounts whose records still report
  `needs rehash` after your chosen deadline.
- Do not re-wrap legacy digests in Argon2id records. The verifier treats the
  two formats as unrelated, so a wrapped record never matches.

## Tests

`password_security_test.go` and `password_parser_security_test.go` cover:

- the argon2id known-answer vector from the reference implementation
  (`phc-winner-argon2` `src/test.c`);
- independent vectors from argon2-cffi;
- new-record round trips, wrong passwords, legacy records (including an
  independently computed SHA-256 fixture) and the rehash signal;
- hostile or malformed parameters, rejected before the KDF runs (checked with a
  derivation counter and a time bound);
- algorithm confusion and cross-format reinterpretation;
- constant-time tag comparison paths, the memory budget, and concurrent use
  under `-race`.

CodeQL could not be run locally. New records no longer pass through
general-purpose digests. The legacy verification branch still does, by design,
until stored legacy records are migrated.
