# Tongsuo certificate constraints (#109 and #113)

Both self-signed issuance and CSR signing now encode the requested CA path length,
including the distinct `MaxPathLen=0, MaxPathLenZero=true` case. Additional
extensions are encoded from validated numeric OIDs and literal hexadecimal DER.
The extension's OID, critical flag and bytes are preserved exactly. Raw caller
bytes are never interpreted as OpenSSL configuration expressions or filenames.

Before returning any certificate, the SM2-capable structured parser verifies the
CA flag, critical basic constraints, exact path length/zero flag, and every
requested additional extension. Existing validity, EKU and name checks remain.
A backend that drops or changes requested fields fails closed.

## Explicit rejection and compatibility

Negative path lengths below -1, inconsistent zero flags, and path constraints on
non-CA certificates are rejected. -1 and an omitted path limit remain unbounded.
Malformed or duplicate extension OIDs and empty raw values are rejected before
issuance. Extra extensions cannot override generated SKI, AKI, key usage, SAN,
basic constraints, certificate policies or EKU; use the corresponding typed
options. Other raw extensions, including custom critical and noncritical
extensions, are supported. These failures replace previously silent omission;
callers must handle them rather than remove a security constraint to proceed.

Native Go issuance is unchanged. Application enrollment authorization and trust
policy remain required. In particular, exact DER fidelity on a self-signed root
does not establish that every relying party enforces trust-anchor path constraints.
This repair addresses lost certificate fields, not the application's CA policy.

## Reproduction and qualification

At `042437b6015f5ba8f185ef91b05b2bd3a5a420f3`, converter assertions and real
Tongsuo 8.5.0 SM2 issuance separately demonstrate dropped `pathlen:0` and a benign
critical custom extension. Equivalent native-Go issuance is a passing control.
After repair, both actual issuance paths retain zero, positive and unbounded
path limits and exact extension bytes. Negative verifier tests independently
reject parseable certificates with missing/changed fields, including a changed
critical flag and changed extension value.

Run `go test -race -count=1 -run 'TestTongsuoConstraints' ./crypto` with a real
`tongsuo` executable on PATH. Tests skip integration when the executable is
absent; such a skip is not evidence that the binary path was qualified.

Review affected certificates already issued by the old backend and reissue those
that lack intended constraints. No database migration is needed. Rolling back
restores omission and must not be combined with continued constrained issuance.

Primary reference: [OpenSSL extension configuration](https://docs.openssl.org/3.5/man5/x509v3_config/).
