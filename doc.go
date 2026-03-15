// Package utils is a comprehensive, production-grade Go utility library
// providing reusable building blocks for cryptography, concurrency,
// file operations, networking, caching, rate limiting, and more.
//
// Requires Go 1.25 or later. Licensed under the MIT License.
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
// # Quick Start
//
// Import the root package for common utilities:
//
//	import gutils "github.com/Laisky/go-utils/v6"
//
// Import sub-packages for domain-specific functionality:
//
//	import (
//	    "github.com/Laisky/go-utils/v6/crypto"
//	    "github.com/Laisky/go-utils/v6/log"
//	    "github.com/Laisky/go-utils/v6/jwt"
//	)
//
// # Root Package
//
// The root package exposes general-purpose utilities organized by topic:
//
//   - File system: [ReplaceFile], [ReplaceFileAtomic], [CopyFile], [MoveFile],
//     [IsDir], [IsFile], [FileExists], [FileMD5], [FileSHA1], [DirSize],
//     [ListFilesInDir], [WatchFileChanging], [RenderTemplate]
//   - HTTP client: [NewHTTPClient] with configurable TLS, proxy, and timeout
//   - Caching: [LRU cache] with TTL support and [TtlCache] for time-based expiration
//   - Rate limiting: [RateLimiter] using a token-bucket algorithm
//   - Concurrency: [Mutex] (atomic-based), [RaceErr], [RaceErrWithCtx],
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
//   - RBAC: [RBACPermKey], [RBACPermFullKey] for role-based access control
//
// # Sub-Packages
//
// The library is organized into focused sub-packages:
//
//   - [github.com/Laisky/go-utils/v6/algorithm] — Data structures including
//     skip lists, FIFO/deque queues, priority queues, and heaps.
//   - [github.com/Laisky/go-utils/v6/common] — Shared type constraints
//     ([Number], [Sortable]) and math helpers ([Min], [Max], [Round],
//     [HumanReadableByteCount], [Number2Roman]).
//   - [github.com/Laisky/go-utils/v6/compress] — Gzip, parallel gzip (pgzip),
//     and ZIP archive operations with decompression-bomb protection.
//   - [github.com/Laisky/go-utils/v6/counter] — Thread-safe counters:
//     atomic [Counter] with speed tracking, [RotateCounter] for circular
//     counting, and [ParallelCounter] for distributed counting.
//   - [github.com/Laisky/go-utils/v6/crypto] — Cryptographic toolkit covering
//     AES-GCM encryption, RSA (PKCS#1 v1.5 and OAEP), ECDSA, Ed25519,
//     SM2/SM3/SM4, password hashing, HKDF, OTP, and X.509 certificate
//     management.
//   - [github.com/Laisky/go-utils/v6/crypto/kms] — Key Management System
//     interface with an in-memory implementation.
//   - [github.com/Laisky/go-utils/v6/crypto/threshold] — Threshold
//     cryptography including Shamir secret sharing and threshold RSA signatures.
//   - [github.com/Laisky/go-utils/v6/email] — SMTP email sending via the
//     [Mail] interface.
//   - [github.com/Laisky/go-utils/v6/gorm] — GORM database helpers including
//     [GzText] for transparent gzip-compressed text storage.
//   - [github.com/Laisky/go-utils/v6/json] — JSON encoding/decoding utilities
//     with comment support.
//   - [github.com/Laisky/go-utils/v6/jwt] — JWT signing and parsing for
//     HS256, ES256, and RS256 algorithms.
//   - [github.com/Laisky/go-utils/v6/log] — Structured logging built on
//     [zap] with sampled logging, log rotation, and alert integration.
//   - [github.com/Laisky/go-utils/v6/agents] — Agent infrastructure for
//     AI/ML applications, including memory management and file storage
//     abstractions.
//
// # CLI Tool (gutils)
//
// The gutils command-line tool exposes several handy operations:
//
//	# find and delete duplicate files or similar images
//	gutils remove-dup examples/images --dry
//
//	# move files into hash-based hierarchical directories
//	gutils md5dir -i examples/md5dir/ --dry
//
//	# show X.509 certificate details
//	gutils certinfo -r blog.laisky.com:443
//	gutils certinfo -f ./cert.pem
//
//	# encrypt a file with AES
//	gutils encrypt aes -i <file_path> -s <password>
//
//	# sign or verify with RSA
//	gutils rsa sign
//	gutils rsa verify
//
// [zap]: https://pkg.go.dev/go.uber.org/zap
package utils
