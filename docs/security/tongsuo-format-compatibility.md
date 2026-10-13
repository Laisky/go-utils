# Tongsuo format compatibility

`NewPrikeyWithPassword` again returns encrypted SEC1 `EC PRIVATE KEY` with
OpenSSL `Proc-Type` / `DEK-Info` headers. It is deprecated because this legacy
password derivation is weak. `NewPrikeyPKCS8WithPassword` explicitly selects the
existing PBES2/PBKDF2-HMAC-SM3 (600,000 iterations), SM4-CBC PKCS#8 format.
Both use environment-based password passing, stdin for plaintext key data and
separate stderr capture; reverting those process-secrecy fixes is unnecessary.

`SignX509CRL` again returns `X509 CRL` PEM. `SignX509CRLDER` explicitly returns
DER and retains all recent signed-output verification and TBS field checks.
The PEM wrapper calls that same verified implementation, so it does not restore
the old unsafe signing logic. The old name is deprecated for format clarity,
not because PEM encoding itself is unsafe.

Existing old-format callers need no data migration. Consumers that adopted
PKCS#8 or DER behavior from the intervening revisions must explicitly select the
new methods. Keep reader support during rollout and rollback. No existing keys,
certificates or CRLs are rewritten by this change. A failed operation returns no
key or CRL bytes. Existing PBES2, wrong-password, stderr separation and DER CRL
verification regressions remain on the appropriate explicit APIs.
