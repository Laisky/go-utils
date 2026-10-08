# Go Utils

Reusable Go packages for service infrastructure, cryptography and PKI, file processing, and agent memory — plus the `gutils` command-line toolbox.

[![Tests](https://github.com/Laisky/go-utils/actions/workflows/test.yml/badge.svg?branch=v6)](https://github.com/Laisky/go-utils/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Laisky/go-utils/v6.svg)](https://pkg.go.dev/github.com/Laisky/go-utils/v6)
[![Go version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**[Go compatibility](#requirements-and-versioning) · [Get started](#get-started) · [Package guide](docs/modules.md) · [CLI](#command-line-toolbox) · [Safety and migration](#safety-and-migration) · [Contributing](#development-and-contributing)**

## What is in the toolbox?

Go Utils brings common infrastructure tasks into one versioned Go module. Use the root package for caches, HTTP helpers, concurrency, rate limiting, files, and permissions; import subpackages for more specialized work.

- **Services:** structured logging, expiring caches, token-bucket limits, HTTP clients, SMTP, and database field helpers.
- **Security:** authenticated encryption, signatures, password hashing, X.509/CSR utilities, JWT verification, and explicit RBAC grants.
- **Files and data:** gzip/ZIP operations, hashing, text templates, generic data structures, and a file-management CLI.
- **Agents:** file-storage interfaces, local and MCP backends, conversation history, tiered memory, and context preparation.

This is a library and toolbox, not an application framework, hosted KMS, authentication server, or complete agent runtime. Import the packages you need; they share a module and may bring transitive dependencies. Prefer the standard library when it already covers your use case.

## Get started

### Requirements and versioning

Choose the release line that matches your Go toolchain:

| Version | Branch | Supported Go version |
| --- | --- | --- |
| v1 | [`master`](https://github.com/Laisky/go-utils/tree/master) | >= 1.16 |
| v2 | [`v2`](https://github.com/Laisky/go-utils/tree/v2) | >= 1.18 |
| v3 | [`v3`](https://github.com/Laisky/go-utils/tree/v3) | >= 1.19 |
| v4 | [`v4`](https://github.com/Laisky/go-utils/tree/v4) | >= 1.21 |
| v5 | [`v5`](https://github.com/Laisky/go-utils/tree/v5) | >= 1.23 |
| v6 | [`v6`](https://github.com/Laisky/go-utils/tree/v6) | >= 1.26 |

These are the project's documented compatibility baselines, not a guarantee of ongoing maintenance for older release lines. Check the selected tag's `go.mod` for its exact toolchain requirements.

The `v6` branch declares **Go 1.26.0 or newer** in [go.mod](go.mod). Use a security-patched toolchain; the minimum language version is not a recommendation to deploy an old patch release.

All v6 imports include `/v6`. This README and the package guide describe the **current `v6` branch**, which may contain changes not yet included in a tagged release. Check the [release history](https://github.com/Laisky/go-utils/releases), [tags](https://github.com/Laisky/go-utils/tags), and [changelog](CHANGELOG.md) before upgrading.

### Install the library

Inside your own Go module:

```sh
go get github.com/Laisky/go-utils/v6@latest
```

`@latest` follows Go's version selection rules; it is not a promise to install the current branch head. For an unreleased API, replace `COMMIT_SHA` in `go get github.com/Laisky/go-utils/v6@COMMIT_SHA` with a reviewed commit from `v6`. Let Go generate any required pseudo-version. Pin the version you have tested and commit `go.mod` and `go.sum`.

### A complete first example

Save this as `main.go` in your application and run `go run .`:

```go
package main

import (
	"fmt"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
)

// main stores and reads a typed value, then stops the cache's cleanup worker.
func main() {
	cache := gutils.NewTtlCache[string]()
	defer cache.Close()

	cache.Set("greeting", "hello, Go", time.Minute)
	value, ok := cache.Get("greeting")
	fmt.Println(value, ok)
}
```

Expected output: `hello, Go true`. Stop users of the cache before closing it; operations after `Close` panic. More examples, including imports and error handling, are in the [package guide](docs/modules.md).

## Find the right package

The entries below are **packages within one Go module**, not independently versioned modules. Paths are relative to `github.com/Laisky/go-utils/v6`.

| Package / area | Core functionality | Start here |
| --- | --- | --- |
| Root: files and templates | Copy/move/replace files, inspect directories, watch changes, hash content, and render Go text templates. | [`RenderTemplate` and file helpers](docs/modules.md#files-and-text-templates) |
| Root: caches | Typed TTL caches, a single-item cache, context-owned expiration, and a bounded SIEVE-backed cache wrapper. | [`NewTtlCache`, `NewExpCache`, `NewLruCache`](docs/modules.md#caches) |
| Root: HTTP | Configurable clients, explicit proxy policy, JSON requests, and replayable request-body construction. | [`NewHTTPClient`, `NewReusableRequest`](docs/modules.md#http) |
| Root: concurrency and rate limiting | Task coordination, cancellation helpers, locks, token consumption, and pluggable limiter state. | [`NewRateLimiter` and coordination helpers](docs/modules.md#concurrency-and-rate-limiting) |
| Root: small utilities | Secure/non-secure randomness, hashing, jump hashing, time, sorting, terminal input, and networking helpers. | [Utility selection and limits](docs/modules.md#other-root-utilities) |
| Root: RBAC | Permission trees, explicit subtree grants, intersection, and conservative revocation. | [Grant semantics and migration](docs/modules.md#rbac) |
| `algorithm` | Generic priority queues, heaps, queues/deques, skip lists, and array/search helpers. | [Priority-queue example](docs/modules.md#algorithm) |
| `common` | Shared numeric/sortable constraints, ordering, numeric and formatting helpers. | [Typed constraints](docs/modules.md#common) |
| `counter` | Atomic counters, speed sampling, rotating counters, and range allocation to child counters. | [Counting example](docs/modules.md#counter) |
| `compress` | Gzip/parallel-gzip compression and ZIP creation/extraction with configurable limits. | [Compression and extraction budgets](docs/modules.md#compress) |
| `crypto` | AES-GCM, RSA/ECDSA/Ed25519, password hashing, derivation, OTP, X.509, and SM-family/Tongsuo utilities. | [Authenticated-encryption example](docs/modules.md#crypto) |
| `crypto/kms`, `crypto/kms/mem` | A key-management interface and an in-process multi-key implementation. | [Key lifecycle and encryption](docs/modules.md#kms) |
| `crypto/threshold/shamir` | Split a secret into shares and reconstruct it from a sufficient subset. | [Share creation and recovery](docs/modules.md#shamir-secret-sharing) |
| `crypto/threshold/signature` | Generate threshold RSA key shares, combine signatures, and verify them. | [Threshold-signature workflow](docs/modules.md#threshold-signatures) |
| `jwt` | HS256/ES256 signing and verification; RS256 verification; per-operation key selection. | [Sign and verify claims](docs/modules.md#jwt) |
| `log` | Structured Zap logging, runtime levels, sampling, UTC daily rotation, and retention. | [Logger setup](docs/modules.md#log) |
| `email` | SMTP delivery with verified TLS by default and explicit transport options. | [Send a notification](docs/modules.md#email) |
| `json` | JSON encoding/decoding helpers, including string output and comment-aware input helpers. | [JSON usage](docs/modules.md#json) |
| `gorm` | Gzip/JSON database field types and metadata-first diagnostic logging. | [Database value conversion](docs/modules.md#gorm) |
| `agents/files` | Low-level MCP FileIO client and pluggable agent file-storage interface. | [Direct FileIO access](docs/modules.md#agent-fileio) |
| `agents/memory/storage`, `/local`, `/mcp` | Standardized storage operations with local filesystem and MCP implementations. | [Storage backend selection](docs/modules.md#agent-storage) |
| `agents/memory` | Turn preparation/persistence, tiered facts, compaction, consolidation, and maintenance. | [`BeforeTurn` / `AfterTurn`](docs/modules.md#agent-memory) |
| `cmd`, `cmd/gutils` | Reusable Cobra commands and the installable CLI entry point. | [CLI usage](#command-line-toolbox) |

`crypto/threshold` is the umbrella documentation package; its implementations live in the two subpackages above. `agents/internal` and `internal` are implementation details, not public integration APIs. Configuration management previously listed here has moved to [go-config](https://github.com/Laisky/go-config).

## Command-line toolbox

### Install

```sh
go install github.com/Laisky/go-utils/v6/cmd/gutils@latest
```

Go installs the executable in `GOBIN`, or the first `GOPATH` entry's `bin` directory when `GOBIN` is unset. Add that directory to your `PATH`, then run `gutils --help`.

To build the implementation described by this branch instead of a published version:

```sh
git clone --branch v6 https://github.com/Laisky/go-utils.git
cd go-utils
go install ./cmd/gutils
```

### Inspect before changing files

```sh
# Preview duplicate-file removal. Review the output before omitting --dry.
gutils remove-dup -d ./photos --dry

# Inspect a certificate file or a remote TLS endpoint.
gutils certinfo -f ./cert.pem
gutils certinfo -r example.com:443

# Move files into MD5-based directories. Run only on a backed-up working copy.
gutils md5dir -i ./working-copy

# Sign a file; verification reads the matching artifact.bin.sig sidecar.
gutils rsa sign --prikey ./private.pem --file ./artifact.bin
gutils rsa verify --pubkey ./public.pem --file ./artifact.bin
```

`remove-dup` and `md5dir` can delete or move files. Do not use your only copy as input, and do not treat image similarity as proof that files are interchangeable. MD5-based organization is not a cryptographic integrity check. Use each subcommand's `--help` to review its current flags.

### Encrypt without exposing a password in arguments

Provision a password file outside the repository through your secret-management process. On Unix, it must not be accessible by group or others:

```sh
chmod 600 ./password.txt

# Writes config.toml.enc.
gutils encrypt aes -i ./config.toml --password-file ./password.txt

# Use a different output path to preserve the original configuration.
gutils decrypt aes -i ./config.toml.enc -o ./config.restored.toml --password-file ./password.txt
```

Password mode derives a key with Argon2id and writes a versioned encrypted format. `--password-file -` reads from standard input. `--key-file` instead accepts a hex-encoded raw AES key; it is not a password input. With neither flag, an interactive terminal prompts without echo; a noninteractive invocation fails.

**`-s/--secret` is no longer accepted.** Existing regular output files can be replaced, so select output paths deliberately. See the [CLI guide](cmd/README.md), [AES formats and migration](crypto/aes_safety.md), and [output-path safety notes](cmd/output_safety.md).

## Safety and migration

| Boundary | What callers need to do |
| --- | --- |
| Keys and cryptographic formats | Use random or properly derived keys, protect them outside ciphertext storage, and use matching AAD for authenticated encryption. Review the [AES](crypto/aes_safety.md) and [Ed25519](crypto/ed25519_migration.md) migration notes before reading older data. |
| RBAC persistence | Preserve explicit `Grant` state. Upgrade readers and writers together; old readers can misinterpret new trees. Rebuild ambiguous legacy policies from trusted assignments. Read the [RBAC migration guide](docs/rbac-grant-migration.md). |
| Untrusted files and archives | Set finite input/output budgets, deadlines, and isolation appropriate to your workload. ZIP extraction is not an archive-wide transaction. Read [extraction](compress/unzip_safety.md), [ZIP source](compress/zip_source_safety.md), and [file hashing](file_hash_safety.md) limits. |
| Network and credentials | Keep TLS verification enabled. Configure proxies and destinations deliberately. SMTP plaintext requires `WithEmailInsecureAllowPlaintext`; a custom transport owns its own security policy. |
| Logs and SQL | Never log credentials or raw tokens. SQL diagnostics omit query content by default; `WithUnsafeSQLLogging` explicitly exposes sensitive text. See the [logging](docs/manual/logs.md) and [GORM](gorm/README.md) guides. |
| Agent data | Authorize project/session access in your application, protect memory as sensitive data, and understand what is sent to a configured MCP or heuristic-model endpoint. Local storage is not automatically encrypted. |

Security-oriented checks and regression tests do not constitute an independent audit or a guarantee that every use is safe. Deprecated and compatibility APIs remain in the repository; the [API reference](https://pkg.go.dev/github.com/Laisky/go-utils/v6) documents individual contracts. Use [command safety guidance](command_safety.md) when launching subprocesses.

## Development and contributing

Start from `v6`. Include a minimal, sanitized reproducer for a bug and preserve it as a behavior/regression test. Keep examples, public API documentation, and migration guidance synchronized with changes.

The [main workflow](.github/workflows/test.yml) builds and runs race-enabled tests on Ubuntu with the latest Go 1.26 patch and the current stable Go release. It also checks formatting, lint, module tidiness, vulnerabilities, and `go vet` for Linux, Windows, and macOS targets. Cross-target vetting is not the same as running the test suite on every OS.

```sh
# Install goimports and govulncheck; install the linter used by the workflow separately.
make install
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

# make lint formats files and runs go mod tidy: inspect the resulting diff.
make lint
go test -race -cover ./...
```

Tongsuo-backed crypto integration tests need the `tongsuo` executable. CI builds the version pinned in its workflow; tests that depend on it are skipped when it is absent locally. A run with skipped integrations is not equivalent to the full CI environment. Threshold RSA key generation can also make the suite slow; see its [testing notes](crypto/threshold/signature/TESTING.md).

Read [AGENTS.md](AGENTS.md) for repository-specific development instructions. Open a [GitHub issue](https://github.com/Laisky/go-utils/issues) for questions or bugs, and a [pull request](https://github.com/Laisky/go-utils/pulls) for changes. Never include live credentials, private keys, or sensitive production data in a public report.

## Further reading

[Package recipes](docs/modules.md) · [API reference](https://pkg.go.dev/github.com/Laisky/go-utils/v6) · [Agent memory manual](docs/manual/agents_memory.md) · [Agent memory architecture](docs/arch/agents_memory_v2.md) · [MCP FileIO reference](docs/ref/mcp_fileio.md) · [Logging guide](docs/manual/logs.md) · [Changelog](CHANGELOG.md)

## License

[MIT](LICENSE). Maintained by [Laisky and contributors](https://github.com/Laisky/go-utils/graphs/contributors).
