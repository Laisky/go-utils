// Package utils provides reusable Go building blocks for cryptography,
// concurrency, file operations, networking, caching, and rate limiting.
//
// Requires Go 1.26.0 or later. Use a security-patched toolchain.
// Licensed under the MIT License.
//
// # Installation
//
// Use as a library:
//
//	go get github.com/Laisky/go-utils/v6@latest
//
// Use the CLI tool:
//
//	go install github.com/Laisky/go-utils/v6/cmd/gutils@latest
//
// All v6 import paths include /v6. The v6 development branch may contain APIs
// not yet included in a release; pin a tested tag or reviewed commit rather
// than assuming @latest is the current branch head.
//
// # Quick Start
//
// Import the root package for common utilities:
//
//	import gutils "github.com/Laisky/go-utils/v6"
//
// Import subpackages for domain-specific functionality:
//
//	import (
//	    "github.com/Laisky/go-utils/v6/crypto"
//	    "github.com/Laisky/go-utils/v6/log"
//	    "github.com/Laisky/go-utils/v6/jwt"
//	)
//
// See the [package guide] for usage recipes, imports, lifecycle requirements,
// and migration notes. Examples and API contracts describe the checked-out
// version, not every historical release.
//
// # Root Package
//
// The root package exposes general-purpose utilities organized by topic:
//
//   - File system: [ReplaceFile], [ReplaceFileAtomic], [CopyFile], [MoveFile],
//     [IsDir], [IsFile], [FileExists], [FileMD5], [FileSHA1], [DirSize],
//     [ListFilesInDir], [WatchFileChanging], [RenderTemplate]
//   - HTTP client: [NewHTTPClient] with configurable TLS, proxy, and timeout
//   - Caching: [NewLruCache], a SIEVE-backed bounded cache wrapper, and
//     [TtlCache] for per-entry expiration
//   - Rate limiting: [RateLimiter] using a token-bucket algorithm
//   - Concurrency: [Mutex], [RaceErr], [RaceErrWithCtx],
//     [RunWithTimeout], [WaitComplete]
//   - Async tasks: [AsyncTaskInterface] and [AsyncTaskResult]
//   - Hashing: [Hash] and [FileHash] supporting SHA-256, SHA-512, and xxhash
//   - Random: [RandomStringWithLength], [SecRandomStringWithLength],
//     [RandomBytesWithLength], [SecRandomBytesWithLength], [RandomChoice]
//   - Consistent hashing: [JumpHash] for bucket assignment
//   - Sorting: generic sort helpers
//   - Networking: TCP/UDP utilities
//   - Terminal: [Input], [InputYes] for interactive prompts
//   - Time: [UTCNow], [TimeEqual] with tolerance comparison
//   - ANSI color: [Color] for terminal output
//   - RBAC: [RBACPermKey], [RBACPermFullKey], and explicit permission grants
//
// Close caches and other owned resources during shutdown. Cancellation and
// timeouts require cooperating workers; they do not forcibly stop arbitrary
// code. RenderTemplate uses text/template, not HTML contextual escaping.
//
// # Subpackages
//
// The library is one Go module containing focused subpackages:
//
//   - github.com/Laisky/go-utils/v6/algorithm: generic queues, heaps,
//     priority queues, skip lists, and array/search helpers.
//   - github.com/Laisky/go-utils/v6/common: shared numeric/sortable constraints,
//     ordering constants, and numeric/formatting helpers.
//   - github.com/Laisky/go-utils/v6/compress: gzip, parallel gzip, and ZIP
//     operations with configurable output budgets.
//   - github.com/Laisky/go-utils/v6/counter: atomic counters, speed sampling,
//     rotation, and in-process range allocation to child counters.
//   - github.com/Laisky/go-utils/v6/crypto: AES-GCM, RSA, ECDSA, Ed25519,
//     SM2/SM3/SM4, password hashing, HKDF, OTP, and X.509/CSR utilities.
//   - github.com/Laisky/go-utils/v6/crypto/kms: a key-management interface;
//     crypto/kms/mem supplies an in-memory implementation, not an HSM.
//   - github.com/Laisky/go-utils/v6/crypto/threshold/shamir: secret sharing.
//   - github.com/Laisky/go-utils/v6/crypto/threshold/signature: threshold RSA.
//   - github.com/Laisky/go-utils/v6/email: SMTP delivery with verified TLS by
//     default and explicit transport customization.
//   - github.com/Laisky/go-utils/v6/gorm: compressed/JSON database fields and
//     metadata-first SQL diagnostic logging.
//   - github.com/Laisky/go-utils/v6/json: JSON helpers, with explicit
//     comment-aware decoding APIs.
//   - github.com/Laisky/go-utils/v6/jwt: HS256/ES256 signing and verification,
//     plus RS256 verification.
//   - github.com/Laisky/go-utils/v6/log: structured logging built on the
//     [Laisky Zap fork], sampling, UTC daily rotation, and retention.
//   - github.com/Laisky/go-utils/v6/agents/files: MCP FileIO and storage APIs.
//   - github.com/Laisky/go-utils/v6/agents/memory/storage: a storage contract
//     with local and MCP implementations in its subpackages.
//   - github.com/Laisky/go-utils/v6/agents/memory: turn preparation and
//     persistence, tiered facts, compaction, and maintenance.
//
// # CLI Tool (gutils)
//
// Preview destructive work and keep backups before changing files:
//
//	# preview duplicate-file removal
//	gutils remove-dup -d ./photos --dry
//
//	# move a backed-up working copy into MD5-based directories
//	gutils md5dir -i ./working-copy
//
//	# inspect an X.509 certificate
//	gutils certinfo -r example.com:443
//	gutils certinfo -f ./cert.pem
//
//	# use a provisioned password file; never put passwords in arguments
//	chmod 600 ./password.txt
//	gutils encrypt aes -i ./config.toml --password-file ./password.txt
//
//	# sign or verify a file and its .sig sidecar
//	gutils rsa sign --prikey ./private.pem --file ./artifact.bin
//	gutils rsa verify --pubkey ./public.pem --file ./artifact.bin
//
// The removed -s/--secret flag is not accepted. Existing regular output files
// can be replaced. Read the CLI and migration guides before processing older
// ciphertext or permission data.
//
// [package guide]: https://github.com/Laisky/go-utils/blob/v6/docs/modules.md
// [Laisky Zap fork]: https://pkg.go.dev/github.com/Laisky/zap
package utils
