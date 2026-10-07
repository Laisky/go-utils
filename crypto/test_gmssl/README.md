# GmSSL / Tongsuo interoperability tests

This directory is a separate Go module (`test_gmssl`). It cross-checks the
go-utils `crypto.Tongsuo` wrapper against independent SM2/SM3/SM4
implementations through cgo:

- the GmSSL C library, through [GmSSL-Go](https://github.com/GmSSL/GmSSL-Go);
- the Tongsuo C library, through [tongsuo-go-sdk](https://github.com/Tongsuo-Project/tongsuo-go-sdk).

`go.mod` replaces `github.com/Laisky/go-utils/v6` with `../..`, so the tests
always exercise the current working tree. Because the module has its own
`go.mod`, the parent's `go build ./...`, `go vet ./...`, `go test ./...` and
golangci-lint never include it.

## What each test checks

| Test | Cross-check |
| --- | --- |
| `Test_HashBySm3` | `HashBySm3` (tongsuo binary) equals GmSSL SM3 for 0, 1, 55, 56, 64 bytes and 1 MiB. |
| `TestTongsuo_SignBySM2SM3` | GmSSL SM2/SM3 signatures verify with `VerifyBySm2Sm3`; `SignBySm2Sm3` signatures verify with GmSSL. Both sides reject a modified signature and a modified message. |
| `TestTongsuo_EncryptBySm4CbcBaisc` | The basic API ciphertext is plain SM4-CBC/PKCS#7 in both directions with GmSSL. The returned tag equals the HMAC-SHA256 over `iv \|\| ciphertext` described in `crypto/smtongsuo.md`, computed independently. A tag that does not match the IV is rejected. |
| `TestTongsuo_Sm4CbcEnvelopeInterop` | The `"GUS4"` v1 envelope from `EncryptBySm4Cbc` decrypts with GmSSL plus standard-library HKDF/HMAC, using only the documented format. Envelopes built that way decrypt with `DecryptBySm4Cbc`. A modified IV is rejected. |
| `TestTongsuo_NewPrikeyWithPassword` | tongsuo-go-sdk decrypts the encrypted key (wrong password rejected); the decrypted key matches `Prikey2Pubkey` and its `SignBySm2Sm3` signature verifies in GmSSL. GmSSL is expected to **reject** the key (see below). |

Known incompatibility: `NewPrikeyWithPassword` writes OpenSSL "traditional"
PEM encryption (`EC PRIVATE KEY` with `Proc-Type`/`DEK-Info: SM4-CBC`). GmSSL
3.1 only imports PKCS#8 `ENCRYPTED PRIVATE KEY` (PBKDF2-HMAC-SM3 + SM4-CBC).
The test checks both the rejection and a GmSSL PKCS#8 positive control.

## Prerequisites

- Go matching the parent module (`go 1.26.5` or newer) and a C toolchain (cgo).
- Tongsuo 8.5.x: the `tongsuo` binary on `PATH`, plus its headers and
  `libcrypto` (tested with Tongsuo 8.5.0 installed in `/opt/tongsuo`).
- GmSSL 3.1.1, the release GmSSL-Go v1.3.1 is built against. Install it into a
  private prefix; root access is not required:

```sh
export GMSSL_PREFIX="$HOME/.local/gmssl-3.1.1"
git clone --depth 1 --branch v3.1.1 https://github.com/guanzhi/GmSSL.git
cmake -S GmSSL -B GmSSL/build -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$GMSSL_PREFIX"
cmake --build GmSSL/build -j"$(nproc)"
cmake --install GmSSL/build
```

tongsuo-go-sdk is pinned to the December 2023 revision. Its v1.0.0 release
includes `<openssl/sm4.h>`, which Tongsuo only installs when it is built with
`enable-export-sm4`, so v1.0.0 does not compile against a default Tongsuo
build.

## Running

```sh
export TONGSUO_PREFIX=/opt/tongsuo
export PATH="$TONGSUO_PREFIX/bin:$PATH"
export CGO_ENABLED=1
export CGO_CFLAGS="-I$GMSSL_PREFIX/include -I$TONGSUO_PREFIX/include"
export CGO_LDFLAGS="-L$GMSSL_PREFIX/lib -Wl,-rpath,$GMSSL_PREFIX/lib -L$TONGSUO_PREFIX/lib -Wl,-rpath,$TONGSUO_PREFIX/lib"

cd crypto/test_gmssl
go vet ./...
go test -race -count=1 -v ./...
```

The tests are skipped (reported as `SKIP`) when `tongsuo` is not on `PATH`.
Both C libraries are still needed to compile the package.

`libgmssl` and Tongsuo's `libcrypto` both export `OPENSSL_hexchar2int` and
`OPENSSL_hexstr2buf`, so `libcrypto` may bind to GmSSL's copies at run time.
They have compatible signatures and decode valid hex the same way. GmSSL's
copies use plain `malloc`/`free` and set no OpenSSL error codes, which is
harmless here. Keep this in mind if a test starts failing inside `libcrypto`
hex parsing.

When this module is linted with the parent `.golangci.lint.yml`,
`gomoddirectives` reports the local `replace`. That `replace` is intentional.
