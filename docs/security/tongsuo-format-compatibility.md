# Tongsuo format compatibility

`NewPrikeyWithPassword` again emits the historical encrypted `EC PRIVATE KEY`
PEM block with Proc-Type/DEK-Info headers. It is deprecated because the legacy
PEM KDF is a single MD5 pass. It still avoids password-in-argv and mixed
stdout/stderr output. `NewPrikeyWithPasswordPKCS8` explicitly opts into the
existing PBES2/PBKDF2-HMAC-SM3/SM4-CBC format, with 600,000 iterations and a
random 16-byte salt. Existing EC PRIVATE KEY consumers need no format migration
to upgrade. Consumers adopting the changed PKCS#8 format must choose its explicit
new name. Keep format-capable readers for any keys already written during that window.

`SignX509CRL` again returns X509 CRL PEM, despite its historical result variable
having been called DER. `SignX509CRLDER` is the explicit DER replacement. Both
retain the repaired signing algorithm, verified signature, exact field preservation,
input size limits, invalid/trailing-data rejection and safe subprocess handling.
This does not restore the old broken SM2 signing implementation: compatibility is
restored by encoding the fully verified DER result as PEM at the old boundary.

Tests first require the old names' PEM contracts and fail before this correction.
Legacy keys are parsed/decrypted through real Tongsuo; CRLs are decoded as PEM,
parsed, signature-verified and compared with original fields. The PKCS#8 tests
retain independent gmsm parsing, KDF bounds, incorrect-password rejection and
secret-handling assertions on the explicit new API. No database rewrite or
production key/certificate operation is performed. Rollback must retain a reader
for every format already stored; never infer trusted formats from decryption failures.
