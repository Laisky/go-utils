# Password record compatibility and explicit Argon2id migration

`PasswordHash(password, hasher)` preserves the selected SHA-256/SHA-512 algorithm,
four-part legacy record, 256/512-byte salt, random iteration range and minimum
delay. It is deprecated but remains usable by existing callers. New writes from
this API continue to verify on old digest-only readers. A library update must not
silently change stored password formats, including during rolling upgrades.

`PasswordHashArgon2id(password)` is the explicit replacement. It emits the same
Argon2id PHC format introduced by the incompatible writer change, so existing
PHC records remain readable. Its parameters are unchanged: 64 MiB, three passes,
four lanes, 16-byte salt and 32-byte tag. The legacy writer is not memory-hard;
deprecation documents this limitation instead of silently changing its algorithm.

## New opt-in records

```
$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
```

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
3. If `needs` is true, call `PasswordHashArgon2id(pw)` and replace
   the stored record. Only do this after a successful verification.

Rollout policy:

- Records explicitly written by PasswordHashArgon2id cannot be verified by older
  digest-only releases; records from PasswordHash remain compatible. Upgrade every service that verifies passwords before any of them
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

The deprecated writer and legacy verifier retain general-purpose digests for
compatibility. The explicit Argon2id writer is the stronger replacement. Preserve
a PHC-capable rollback reader once opting into that format; never rehash unless
password verification succeeded, and never downgrade a PHC record to aid rollback.
