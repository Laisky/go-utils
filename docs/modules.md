# Package guide and usage recipes

[Back to README](../README.md)

This guide describes the current `v6` branch. Install a version containing the APIs you use; the newest tag and branch head need not be identical. The [README](../README.md#get-started) explains installation and version pinning.

The Go snippets below are **function bodies**, unless stated otherwise. Put one inside `func main()` and add the imports listed with it. They use `panic(err)` to stop a small demonstration on failure; reusable application code should return or handle errors at its boundary. Snippets that access files, SMTP, HTTP, or MCP require the stated configuration and are not sandboxed simulations.

## Root package

Import the root as `gutils "github.com/Laisky/go-utils/v6"`. Its declared package name is `utils`; the alias makes call sites easier to distinguish from application utilities.

### Files and text templates

**Core functionality:** file inspection and traversal, copy/move/replace operations, change watching, file hashes, and rendering a string with Go's `text/template` syntax.

**Use it for:** configuration text, small generated text files, and routine filesystem work. `RenderTemplate` is the text-template helper; this repository is not a catalog of generated application-project templates.

Imports: `"fmt"` and `gutils "github.com/Laisky/go-utils/v6"`.

```go
rendered, err := gutils.RenderTemplate(
	"service={{.Name}}\nport={{.Port}}\n",
	struct {
		Name string
		Port int
	}{Name: "worker", Port: 8080},
)
if err != nil {
	panic(err)
}
fmt.Print(string(rendered))
```

This produces `service=worker` and `port=8080` on separate lines. Keep template source trusted. `text/template` does not provide HTML contextual escaping; use `html/template` for HTML and validate/encode values for the destination format. Rendering text does not make it safe to execute as shell or SQL.

For filesystem operations, select `CopyFile`, `MoveFile`, `ReplaceFile`, or `ReplaceFileAtomic` according to whether you need a copy, relocation, or replacement. Read the individual overwrite and permission contracts instead of assuming all helpers are transactional. `ListFilesInDir` and `WatchFileChanging` cover discovery and change observation; hashing limits are separate from these operations.

[Implementation](../fs.go) · [Examples/tests](../fs_test.go) · [Hashing limits](../file_hash_safety.md) · [Subprocess safety](../command_safety.md)

### Caches

**Core functionality:** generic in-memory caches with different eviction and ownership models.

| Constructor | Choose it when | Lifecycle / caveat |
| --- | --- | --- |
| `NewTtlCache[T]()` | Each entry needs its own TTL. | `Set`, `Get`, `Delete`; call `Close` after stopping callers. Access after close panics. |
| `NewExpCache[T](ctx, ttl)` | Entries share a TTL and the owner already has a context. | `Store`, `Load`, `LoadAndDelete`, `Delete`; cancel the context to stop background cleanup. |
| `NewSingleItemExpCache[T](ttl)` | One computed value needs a freshness window. | `Set` and `Get`; no map or cleanup worker to manage. |
| `NewLruCache[K, V](size, ttl)` | Capacity must also be bounded. | Returns the upstream SIEVE cache type. Despite the historical name, do not assume strict LRU eviction semantics. |

Imports: `"fmt"`, `"time"`, and `gutils "github.com/Laisky/go-utils/v6"`.

```go
cache := gutils.NewTtlCache[int]()
defer cache.Close()
cache.Set("jobs-ready", 12, 30*time.Second)
if count, ok := cache.Get("jobs-ready"); ok {
	fmt.Println(count)
}
cache.Delete("jobs-ready")
```

A TTL is not a capacity limit. Bound key cardinality in the application or choose a capacity-bounded cache. Use a positive TTL. These caches are process-local and are not durable stores or distributed coordination mechanisms.

[Implementation](../cache.go) · [Examples/tests](../cache_test.go)

### HTTP

**Core functionality:** `NewHTTPClient` builds a reusable `*http.Client`; `NewReusableRequest` constructs request bodies that can be replayed when their reader supports it. `RequestJSON` and `RequestJSONWithClient` provide higher-level JSON request helpers.

`NewHTTPClient` connects directly by default. Explicitly add `WithHTTPClientProxyFromEnvironment()` to apply environment proxy policy. `WithHTTPClientProxy(...)` selects a fixed trusted proxy and bypasses `NO_PROXY`. The internal default client used by `RequestJSON` separately opts into environment proxies.

Imports: `"context"`, `"fmt"`, `"io"`, `"net/http"`, `"os"`, `"time"`, and `gutils "github.com/Laisky/go-utils/v6"`.

```go
client, err := gutils.NewHTTPClient(
	gutils.WithHTTPClientTimeout(10*time.Second),
	gutils.WithHTTPClientProxyFromEnvironment(),
)
if err != nil {
	panic(err)
}
defer client.CloseIdleConnections()

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
req, err := gutils.NewReusableRequest(ctx, http.MethodGet, "https://example.com", nil)
if err != nil {
	panic(err)
}
resp, err := client.Do(req)
if err != nil {
	panic(err)
}
defer func() {
	if err := resp.Body.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close response:", err)
	}
}()
if resp.StatusCode != http.StatusOK {
	panic(fmt.Errorf("unexpected HTTP status: %s", resp.Status))
}
const maxBody = 1 << 20
body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
if err != nil {
	panic(err)
}
if len(body) > maxBody {
	panic("response exceeds 1 MiB")
}
fmt.Println("received bytes:", len(body))
```

Create and reuse clients at application scope, not once per request. For a replayable body use `*bytes.Buffer`, `*bytes.Reader`, or `*strings.Reader`; arbitrary streams are not automatically buffered for replay. Replayability is not application-level idempotency or an unconditional retry guarantee. Keep TLS verification enabled, and authorize destinations before accepting URLs from untrusted users.

[Implementation](../http.go) · [HTTP/2 background](ref/http2_goaway.md)

### Concurrency and rate limiting

**Core functionality:** task/race/timeout helpers, synchronization primitives, and token-bucket admission control. `RateLimiterStateManager` makes token storage pluggable; the supplied default manager coordinates goroutines in one process.

Imports: `"context"`, `"fmt"`, and `gutils "github.com/Laisky/go-utils/v6"`.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
limiter, err := gutils.NewRateLimiter(ctx, gutils.RateLimiterArgs{
	NPerSec: 10,
	Max:     20,
})
if err != nil {
	panic(err)
}
if limiter.Allow() {
	fmt.Println("request admitted")
} else {
	fmt.Println("request rejected; apply your backpressure policy")
}
```

`NPerSec` must be positive and `Max` must be at least `NPerSec`. The initial token count defaults to `NPerSec`, not `Max`; `WithAvailableTokens` overrides it. `Allow` consumes one token and `AllowN` consumes a requested number. Cancellation owns the refill worker's lifetime. Snapshots/clones are not a shared distributed quota; use an appropriate state-manager implementation and ownership protocol for cross-process coordination.

`RaceErr`, `RaceErrWithCtx`, `RunWithTimeout`, and `WaitComplete` provide other coordination patterns. A timeout cannot forcibly terminate arbitrary Go code: workers must cooperate with cancellation and release their own resources. Prefer standard `sync`/context primitives when their contract is sufficient, and benchmark representative workloads rather than assuming a universal speed advantage.

[Rate limiter](../ratelimiter.go) · [Limiter tests](../ratelimiter_test.go) · [Concurrency helpers](../sync.go) · [Async tasks](../async.go)

### Other root utilities

| Area | Main entry points | Practical use / boundary |
| --- | --- | --- |
| Randomness | `SecRandomBytesWithLength`, `SecRandomStringWithLength`; non-security `Random*` helpers | Use cryptographic randomness for secrets. Do not use ordinary random choices as token/key generation. |
| Hashing | `Hash`, `FileHash`, `FileMD5`, `FileSHA1` | Pick the algorithm deliberately; legacy checksums are not authentication. Limit untrusted file/input work. |
| Bucket assignment | `JumpHash` | Map a key to a bucket; the application still owns membership, migration, and failure handling. |
| Time | `UTCNow`, `TimeEqual`, clock helpers | UTC timestamps and tolerance-based comparison. Use monotonic elapsed-time measurement where duration matters. |
| Sorting and numbers | Root sorting helpers and `common` | Typed collection/numeric convenience; standard `slices` and `cmp` may already suffice. |
| Terminal and networking | `Input`, `InputYes`, `Color`, TCP/UDP helpers | Interactive programs and service plumbing. Do not assume an interactive terminal or log private inputs. |

For a streaming SHA-256 digest, import `"fmt"`, `"strings"`, and `gutils "github.com/Laisky/go-utils/v6"`:

```go
digest, err := gutils.Hash(gutils.HashTypeSha256, strings.NewReader("hello"))
if err != nil {
	panic(err)
}
fmt.Printf("%x\n", digest)
```

[API reference](https://pkg.go.dev/github.com/Laisky/go-utils/v6) · [File-hash safety](../file_hash_safety.md)

### RBAC

**Core functionality:** hierarchical positive grants with `HasPerm2`, semantic intersection, and revocation via `Cut`. The `Grant` field distinguishes a structural node from a node granting its subtree.

Start with `NewPermissionTree()`, populate a policy using explicit `RBACGrantNone` or `RBACGrantSubtree` modes, and evaluate the complete required permission with `HasPerm2`. A new empty tree grants nothing. A subtree grant covers the node and descendants; a key ending in `.*` covers descendants only. Do not silently make the root a subtree grant as a setup shortcut.

| Operation | Meaning |
| --- | --- |
| `Intersection(other)` | Restrict to permissions accepted by both inputs. |
| `OverwriteBy(other, true)` | The same semantic intersection, taking matching display titles from the other tree. |
| `OverwriteBy(other, false)` | Update display titles, not permission identity or grant state. |
| `Cut(key)` | Revoke overlapping grants without expanding access. Cutting inside a broad grant may revoke that entire grant. |

This representation has no deny exceptions, so it cannot represent “everything below X except Y” by retaining an unrestricted X grant. Preserve explicit modes when serializing. Upgrade old readers and writers together, and rebuild legacy records whose intent is ambiguous. Deprecated `HasPerm` is not interchangeable with `HasPerm2`.

[Migration, worked examples, and limitations](rbac-grant-migration.md)

## Algorithm

Import `"github.com/Laisky/go-utils/v6/algorithm"`.

**Core functionality:** queues/deques, priority queues, heaps, skip lists, and array/search helpers. Choose the data structure for its ordering/access contract rather than using a priority queue for FIFO work.

Imports: `"fmt"`, `"github.com/Laisky/go-utils/v6/algorithm"`, and `"github.com/Laisky/go-utils/v6/common"`.

```go
queue := algorithm.NewPriorityQ[int](common.SortOrderAsc)
queue.Push(algorithm.PriorityItem[int]{Val: 20, Name: "later"})
queue.Push(algorithm.PriorityItem[int]{Val: 5, Name: "first"})
for queue.Len() > 0 {
	fmt.Println(queue.Pop().GetVal())
}
```

This prints `5` then `20`. Ascending order returns the smallest value first; descending returns the largest. `Peek` returns nil on an empty queue, but `Pop` must be guarded against emptiness. `PriorityQ` has no internal locking; synchronize shared access yourself. Do not generalize one structure's concurrency guarantees to the whole package.

[Package source and tests](../algorithm) · [Priority queue](../algorithm/priorityq.go)

## Common

Import `"github.com/Laisky/go-utils/v6/common"`.

**Core functionality:** shared `Number` and `Sortable` constraints, `SortOrderAsc`/`SortOrderDesc`, and numeric/formatting helpers such as `Min`, `Max`, `Round`, `HumanReadableByteCount`, and `Number2Roman`.

Use the ordering constants with `algorithm`, as shown above. For your own generic APIs, constrain arithmetic values with `common.Number` and ordered values with `common.Sortable`. For example, this is a **package-level type declaration**, not a function-body recipe:

```go
// Reading associates a numeric measurement with its unit.
type Reading[T common.Number] struct {
	Value T
	Unit  string
}
```

Instantiate it as `Reading[float64]{Value: 12.5, Unit: "ms"}`. These helpers do not attach physical units, precision guarantees, or overflow protection to ordinary numeric types. When using byte/string conversion helpers, follow their ownership and mutation contracts; do not assume a zero-copy view is an independent copy.

[Package source](../common) · [API reference](https://pkg.go.dev/github.com/Laisky/go-utils/v6/common)

## Counter

Import `"github.com/Laisky/go-utils/v6/counter"`.

**Core functionality:** atomic increment/read operations, throughput sampling, rotating counters, and allocating ranges to child counters.

Imports: `"fmt"` and `"github.com/Laisky/go-utils/v6/counter"`.

```go
processed := counter.NewCounter()
processed.CountN(3)
processed.Count()
fmt.Println(processed.Get()) // 4
```

`GetSpeed` measures change since its previous call and updates that sampling baseline. `NewParallelCounter(quoteStep, rotatePoint)` allocates ranges to children obtained with `GetChild`, reducing allocation coordination. It is in-process range allocation, not a durable distributed sequence service. Rotation and multiple children mean you must not assume an indefinitely increasing global order or permanent identifier uniqueness.

[Implementation and examples](../counter)

## Compress

Import `gcompress "github.com/Laisky/go-utils/v6/compress"`.

**Core functionality:** gzip and parallel-gzip writers, convenience compression/decompression, ZIP creation, and bounded ZIP extraction.

Imports: `"bytes"`, `"fmt"`, `"strings"`, and `gcompress "github.com/Laisky/go-utils/v6/compress"`.

```go
var compressed bytes.Buffer
if err := gcompress.GzCompress(strings.NewReader("hello, compression"), &compressed); err != nil {
	panic(err)
}
fmt.Println("compressed bytes:", compressed.Len())
```

For decompression, pass a finite limit such as `WithGzDecompressMaxBytes(8 << 20)` to `GzDecompress(input, output, ...)`. The option requires a positive value; do not rely on zero as an unlimited sentinel. Low-level writers need finalization: their `Flush` finalizes the compressed stream, not merely a reusable transport buffer. Discard partial output after an error.

`Unzip` defaults to 64 MiB total decompressed output, 16 MiB per file, and 100,000 entries. For a larger trusted archive, set both `UnzipWithMaxBytes` and `UnzipWithMaxFileBytes` to explicit budgets. Per-file publication is not an archive-wide transaction: earlier extracted files may remain if a later member fails. Output budgets do not bound compressed-input size, ZIP-directory parsing, or elapsed CPU time; impose outer limits as well.

[Gzip implementation](../compress/compress.go) · [ZIP extraction safety](../compress/unzip_safety.md) · [ZIP source safety](../compress/zip_source_safety.md)

## Crypto

Import `gcrypto "github.com/Laisky/go-utils/v6/crypto"`.

**Core functionality:** authenticated encryption, public-key signatures and conversion, password hashing and key derivation, OTP, X.509/CSR operations, and SM-family/Tongsuo integration.

### Authenticated encryption

Imports: `"crypto/rand"`, `"fmt"`, and `gcrypto "github.com/Laisky/go-utils/v6/crypto"`.

```go
key := make([]byte, 32) // A raw AES-256 key, not a password.
if _, err := rand.Read(key); err != nil {
	panic(err)
}
aad := []byte("tenant=demo;purpose=example")
ciphertext, err := gcrypto.AEADEncrypt(key, []byte("private message"), aad)
if err != nil {
	panic(err)
}
plaintext, err := gcrypto.AEADDecrypt(key, ciphertext, aad)
if err != nil {
	panic(err)
}
fmt.Println(string(plaintext))
```

`AEADEncrypt` creates a random nonce and includes it with the ciphertext and authentication tag. Raw keys must contain 16, 24, or 32 bytes. Additional authenticated data (AAD) is authenticated but not encrypted; decryption requires identical AAD. This helper rejects empty plaintext. A new random key in this demonstration only lives for the run; real applications must securely retain/manage the correct key to read persistent ciphertext.

### Choosing other crypto functionality

| Need | How to use this package safely |
| --- | --- |
| Password-based file encryption | Use the CLI's `--password-file` mode or the password APIs documented in the AES guide. Do not pass password bytes directly as an AES key. |
| Large encrypted streams/files | Select the documented authenticated streaming/file format and its resource limits; do not assume every historical stream API provides authentication. |
| RSA, ECDSA, Ed25519 | Match signing and verification algorithms, key encodings, and prehash conventions. Review the Ed25519 migration guide for existing signatures. |
| X.509 and CSRs | Parse/generate certificates and keys using the PKI APIs. CSR signature validity proves possession, not authorization to request a name or CA capability; apply issuer policy separately. |
| Password hashes, HKDF, OTP, mnemonic helpers | Use the API intended for the primitive, choose its parameters explicitly, and preserve the required metadata. A checksum is not a password hash, and a mnemonic is sensitive key material. |
| SM2/SM3/SM4 and Tongsuo operations | Use the appropriate pure-Go or external-tool API for the format required. External-tool integrations require their executable and environment. |

[Crypto source/API](../crypto) · [AES formats, limits, and migration](../crypto/aes_safety.md) · [Ed25519 migration](../crypto/ed25519_migration.md)

## KMS

Imports: `"github.com/Laisky/go-utils/v6/crypto/kms"` for the interface and `kmsmem "github.com/Laisky/go-utils/v6/crypto/kms/mem"` for the supplied implementation.

**Core functionality:** register key-encryption keys (KEKs), derive data-encryption keys, and encrypt/decrypt values with metadata identifying the relevant key material. The memory implementation chooses the largest KEK ID for new encryption.

Imports for this recipe: `"context"`, `"crypto/rand"`, `"fmt"`, and `kmsmem "github.com/Laisky/go-utils/v6/crypto/kms/mem"`.

```go
kek := make([]byte, 32)
if _, err := rand.Read(kek); err != nil {
	panic(err)
}
manager, err := kmsmem.New(map[uint16][]byte{1: kek})
if err != nil {
	panic(err)
}
ctx := context.Background()
aad := []byte("record:demo")
encrypted, err := manager.Encrypt(ctx, []byte("private record"), aad)
if err != nil {
	panic(err)
}
plaintext, err := manager.Decrypt(ctx, encrypted, aad)
if err != nil {
	panic(err)
}
fmt.Println(string(plaintext))
```

`mem` keeps and exposes key material in process memory. It is **not an HSM, managed cloud KMS, durable key database, or authorization boundary**. Persist keys through a separate secure mechanism, retain old KEKs while their ciphertext still exists, and authorize key IDs before calling derivation/export operations. `AddKek` with a new ID changes the key used for new encryption; it does not automatically migrate old ciphertext.

[Interface](../crypto/kms/interface.go) · [In-memory implementation](../crypto/kms/mem/kms.go)

## Shamir secret sharing

Import `"github.com/Laisky/go-utils/v6/crypto/threshold/shamir"`.

**Core functionality:** divide a secret among custodians and recover it using a sufficient subset. The wrapper requires `2 <= threshold < total < 256` and a nonempty secret.

Imports: `"bytes"`, `"crypto/rand"`, `"fmt"`, and `"github.com/Laisky/go-utils/v6/crypto/threshold/shamir"`.

```go
secret := make([]byte, 32)
if _, err := rand.Read(secret); err != nil {
	panic(err)
}
shares, err := shamir.Split(secret, 5, 3)
if err != nil {
	panic(err)
}
subset := make(map[byte][]byte, 3)
for id, share := range shares {
	subset[id] = share // Preserve both the original ID and its bytes.
	if len(subset) == 3 {
		break
	}
}
recovered, err := shamir.Combine(subset)
if err != nil {
	panic(err)
}
fmt.Println(bytes.Equal(secret, recovered)) // true
```

Store shares separately and protect their metadata and integrity. `Combine` is not an authentication check and does not know the original threshold; supply enough valid shares from the same split and verify the recovered secret through your protocol. This is secret reconstruction, not distributed signing.

[Implementation](../crypto/threshold/shamir/shamir.go) · [Tests](../crypto/threshold/shamir/shamir_test.go)

## Threshold signatures

Import `tsign "github.com/Laisky/go-utils/v6/crypto/threshold/signature"`.

**Core functionality:** generate RSA key shares, produce a combined SHA-256 signature from a quorum, and verify it with the public key. Unlike Shamir recovery, the operation's output is a signature.

Imports: `"strings"`, `gcrypto "github.com/Laisky/go-utils/v6/crypto"`, and `tsign "github.com/Laisky/go-utils/v6/crypto/threshold/signature"`.

```go
shares, meta, err := tsign.NewKeyShares(3, 2, gcrypto.RSAPrikeyBits2048)
if err != nil {
	panic(err)
}
sig, err := tsign.SignBySHA256(strings.NewReader("release manifest"), shares[:2], meta)
if err != nil {
	panic(err)
}
if err := tsign.VerifyBySHA256(strings.NewReader("release manifest"), meta.PublicKey, sig); err != nil {
	panic(err)
}
```

Key generation is expensive; do it during provisioning, not per request. The threshold must be a majority (`total/2 + 1`) through `total`, and at least two. The implementation enforces a 2048-bit RSA floor. The convenience signing function receives key shares in one process; it does not itself provide isolated custodians, authenticated networking, distributed key generation, or a consensus protocol. Those are application responsibilities.

[Implementation](../crypto/threshold/signature/rsa.go) · [Testing and runtime considerations](../crypto/threshold/signature/TESTING.md)

## JWT

Import `gjwt "github.com/Laisky/go-utils/v6/jwt"` and use `"github.com/golang-jwt/jwt/v5"` for claim types.

**Core functionality:** HS256 and ES256 signing/verification, RS256 verification, and per-operation key overrides. `Sign` does not implement RS256 signing merely because the package exports `SignMethodRS256`.

Imports: `"crypto/rand"`, `"fmt"`, `"slices"`, `"time"`, `gjwt "github.com/Laisky/go-utils/v6/jwt"`, and `"github.com/golang-jwt/jwt/v5"`.

```go
secret := make([]byte, gjwt.MinHS256SecretLen)
if _, err := rand.Read(secret); err != nil {
	panic(err)
}
signer, err := gjwt.New(gjwt.WithSecretByte(secret))
if err != nil {
	panic(err)
}
now := time.Now()
token, err := signer.Sign(jwt.RegisteredClaims{
	Issuer:    "example-service",
	Subject:   "user-123",
	Audience:  jwt.ClaimStrings{"example-client"},
	IssuedAt:  jwt.NewNumericDate(now),
	ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
})
if err != nil {
	panic(err)
}
var claims jwt.RegisteredClaims
if err := signer.ParseClaimsByHS256(token, &claims); err != nil {
	panic(err)
}
// Enforce application-required claims, not just the cryptographic signature.
if claims.ExpiresAt == nil || !time.Now().Before(claims.ExpiresAt.Time) ||
	claims.Issuer != "example-service" || claims.Subject == "" ||
	!slices.Contains(claims.Audience, "example-client") {
	panic("token does not satisfy application policy")
}
fmt.Println(claims.Subject)
```

HS256 requires at least 32 secret bytes. Persist/configure a real signing key through your key-management process; the generated example key is ephemeral. Keep algorithm selection trusted and define issuer, audience, expiry, subject, revocation, and authorization policy in the application. `ParseTokenWithoutValidate` only decodes untrusted data; never use it as proof of identity.

[Implementation](../jwt/jwt.go) · [Package tests](../jwt)

## Log

Import `glog "github.com/Laisky/go-utils/v6/log"` and **`"github.com/Laisky/zap"`** for structured fields. This repository uses the Laisky Zap fork; do not mix in field types from another Zap import path.

**Core functionality:** JSON/console logging, scoped fields, runtime log levels, probabilistic sampling, UTC daily file rotation, and retention.

Imports: `"fmt"`, `"os"`, `glog "github.com/Laisky/go-utils/v6/log"`, and `"github.com/Laisky/zap"`.

```go
logger, err := glog.New(
	glog.WithName("worker"),
	glog.WithEncoding(glog.EncodingJSON),
	glog.WithLevel(glog.LevelInfo),
)
if err != nil {
	panic(err)
}
defer func() {
	if err := logger.Sync(); err != nil {
		fmt.Fprintln(os.Stderr, "sync logger:", err)
	}
}()
logger.Info("job completed", zap.String("job_id", "job-123"), zap.Int("items", 12))
```

Use `Named`/`With` for component or request context and `ChangeLevel` for runtime changes. Sampling values use a 1,000-point scale: `InfoSample(100, ...)` samples approximately 10%, not 100%. For file sinks, `WithRotation(path)` selects UTC daily rotation and `WithRotationRetention(7)` keeps seven days; zero retention keeps historical rotations indefinitely. Protect the directory and its ancestors, and avoid logging secrets even in debug mode.

[Full logging manual](manual/logs.md)

## Email

Import `"github.com/Laisky/go-utils/v6/email"`.

**Core functionality:** configure SMTP, set authentication credentials, and send a message with optional transport customization.

Imports: `"os"` and `"github.com/Laisky/go-utils/v6/email"`. Configure `SMTP_HOST`, `SMTP_USERNAME`, and `SMTP_PASSWORD`; replace the example sender/recipient with authorized addresses before running.

```go
host := os.Getenv("SMTP_HOST")
username := os.Getenv("SMTP_USERNAME")
password := os.Getenv("SMTP_PASSWORD")
if host == "" || username == "" || password == "" {
	panic("configure SMTP_HOST, SMTP_USERNAME, and SMTP_PASSWORD")
}
mailer := email.NewMail(host, 587)
mailer.Login(username, password)
if err := mailer.Send(
	"sender@example.com", "recipient@example.com",
	"Example service", "Operator", "Job completed", "The job completed.",
); err != nil {
	panic(err)
}
```

The default transport requires verified TLS: port 465 uses implicit TLS; other ports require STARTTLS. `WithEmailRequireTLS` can provide a custom CA/TLS configuration. `WithEmailInsecureAllowPlaintext` explicitly restores plaintext-capable delivery for a deliberately trusted relay; it is not the default. `WithMailSendDialer` replaces the transport and takes responsibility for its security. This package does not provide a delivery queue or retry/idempotency policy for your application.

[Transport and API details](../email/email.go)

## JSON

Import `gjson "github.com/Laisky/go-utils/v6/json"`.

**Core functionality:** standard JSON marshal/unmarshal wrappers, direct string conversion, and an explicit HuJSON input path for comments/trailing commas.

Imports: `"fmt"` and `gjson "github.com/Laisky/go-utils/v6/json"`.

```go
var config struct {
	Name string `json:"name"`
}
if err := gjson.UnmarshalCommentFromString(`{
	// Human-edited configuration.
	"name": "worker",
}`, &config); err != nil {
	panic(err)
}
encoded, err := gjson.MarshalToString(config)
if err != nil {
	panic(err)
}
fmt.Println(encoded) // {"name":"worker"}
```

`Unmarshal`/`UnmarshalFromString` remain strict JSON; choose `UnmarshalComment`/`UnmarshalCommentFromString` explicitly for human-edited configuration. The byte-slice comment path can mutate its input, so copy shared/read-only buffers first. The package does not validate your application's configuration schema or supply every API in `encoding/json`.

[Encoding](../json/json.go) · [Comment-aware decoding](../json/comment.go)

## GORM

Import `ggorm "github.com/Laisky/go-utils/v6/gorm"`.

**Core functionality:** `GzText` stores compressed text through SQL `Value`/`Scan`; `JSON` provides a database value type; `NewLogger` provides operation-oriented diagnostic logging. These are helpers, not a database driver or a complete ORM.

Imports: `"fmt"` and `ggorm "github.com/Laisky/go-utils/v6/gorm"`.

```go
original := ggorm.GzText("a document to store")
value, err := original.Value()
if err != nil {
	panic(err)
}
var restored ggorm.GzText
if err := restored.Scan(value); err != nil {
	panic(err)
}
fmt.Println(string(restored))
```

Use a compatible binary database column for compressed values. `GzText.Scan` limits decompressed data to 64 MiB; compressed storage is neither encryption nor directly searchable plaintext. Parameterize SQL independently of the logging helper.

`NewLogger(formatter, sink)` omits SQL text, bound parameters, and raw errors by default. `WithUnsafeSQLLogging()` explicitly enables sensitive text; debug level is not a redaction boundary. The historical `/*disable_log*/` marker has no control effect. Review logger interface compatibility when integrating with a particular ORM version.

[Field implementation](../gorm/fields.go) · [SQL diagnostics guide](../gorm/README.md)

## Agent FileIO

Import `agentfiles "github.com/Laisky/go-utils/v6/agents/files"`.

**Core functionality:** the low-level `Storage` abstraction and MCP FileIO adapter. Use this layer when integrating file tools directly; use the standardized memory-storage layer below when building an agent-memory engine.

Imports: `"context"`, `"fmt"`, `"os"`, `"time"`, and `agentfiles "github.com/Laisky/go-utils/v6/agents/files"`. Configure an authorized HTTPS MCP FileIO endpoint and API key.

```go
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
backend, err := agentfiles.NewMCPStorageFromConfig(
	ctx, os.Getenv("MEMORY_MCP_ENDPOINT"), os.Getenv("MEMORY_MCP_API_KEY"),
)
if err != nil {
	panic(err)
}
info, err := backend.Stat(ctx, "demo", "/notes/welcome.txt")
if err != nil {
	panic(err)
}
fmt.Println("exists:", info.Exists)
```

Check operation errors even after construction; constructing a client is not proof that a file or remote service is available. For an existing/custom tool caller, use `NewMCPStorage(MCPStorageConfig{Caller: caller})`. The default direct client requires HTTPS and restricts redirects. These transport checks do not replace project authorization, destination allowlisting, or server-side quotas.

[Storage interface](../agents/files/interface.go) · [MCP adapter](../agents/files/mcp_storage.go) · [FileIO reference](ref/mcp_fileio.md)

## Agent storage

Imports: `"github.com/Laisky/go-utils/v6/agents/memory/storage"`, `"github.com/Laisky/go-utils/v6/agents/memory/storage/local"`, or `mcpstorage "github.com/Laisky/go-utils/v6/agents/memory/storage/mcp"`.

**Core functionality:** one `storage.Engine` contract for local files or MCP FileIO. `storage.FromFilesStorage` adapts an existing `agents/files.Storage` implementation.

| Operation | Usage contract |
| --- | --- |
| `Read(ctx, project, path, offset, length)` | Byte offsets; `length == -1` reads to EOF. A range request is not a guarantee of bounded backend allocation. |
| `Write(ctx, project, path, content, mode, offset)` | `WriteModeAppend` appends; `WriteModeOverwrite` writes at an offset without truncating the tail; `WriteModeTruncate` replaces from zero. |
| `Stat` / `List` | Inspect metadata; `List` returns entries, `hasMore`, and an error. Check truncation rather than assuming a complete listing. |
| `Search` | Search within a project/path prefix, with a result limit. Backend search is not an exhaustive database query. |
| `Delete` | Remove the selected path; choose the recursive flag deliberately. |

Imports for the local recipe: `"context"`, `"fmt"`, `"os"`, `"github.com/Laisky/go-utils/v6/agents/memory/storage"`, and `"github.com/Laisky/go-utils/v6/agents/memory/storage/local"`.

```go
store, err := local.NewEngine(local.Config{RootDir: "./agent-memory"})
if err != nil {
	panic(err)
}
defer func() {
	if err := store.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close storage:", err)
	}
}()
ctx := context.Background()
if err := store.Write(ctx, "demo", "/notes/welcome.txt", "hello",
	storage.WriteModeTruncate, 0); err != nil {
	panic(err)
}
content, err := store.Read(ctx, "demo", "/notes/welcome.txt", 0, -1)
if err != nil {
	panic(err)
}
fmt.Println(content)
```

For remote storage, construct `mcpstorage.NewEngine(ctx, mcpstorage.Config{Endpoint: endpoint, APIKey: apiKey})`, check its error, and pass the result wherever `storage.Engine` is expected. Use a cancellable/deadline context for network operations and a secret manager for the API key.

Projects and paths are validated: use a project identifier containing letters, digits, `_`, `.`, or `-`, and canonical absolute storage paths such as `/notes/welcome.txt`, not filesystem paths from an end user. Your application must still authorize each project/session.

Local storage defaults to owner-only directory/file modes (`0700`/`0600`) for newly created entries and uses a rooted filesystem handle; existing directory permissions remain an operator responsibility. It reads file content into memory for range reads and has bounded local search/list behavior. It is not a transactional database, a multi-process coordination service, or encrypted storage. See the manual's operational limits before choosing it for concurrent production workloads.

[Storage contract](../agents/memory/storage/interface.go) · [Local implementation](../agents/memory/storage/local/engine.go) · [Backend configuration and limits](manual/agents_memory.md)

## Agent memory

Import `"github.com/Laisky/go-utils/v6/agents/memory"` and a storage backend.

**Core functionality:** prepare a model's input with relevant memory, persist turn history, maintain tiered facts, compact context, consolidate insights, and run maintenance. This package does not itself replace your application/model call loop.

The lifecycle is **`BeforeTurn` → your model call → `AfterTurn`**. The following single-message example uses a fixed output instead of making a model request. Run it on a fresh demo session; real applications must generate stable, unique turn IDs and reuse an ID only when retrying that same turn.

Imports: `"context"`, `"fmt"`, `"os"`, `"github.com/Laisky/go-utils/v6/agents/memory"`, and `"github.com/Laisky/go-utils/v6/agents/memory/storage/local"`.

```go
store, err := local.NewEngine(local.Config{RootDir: "./agent-memory-demo"})
if err != nil {
	panic(err)
}
defer func() {
	if err := store.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close storage:", err)
	}
}()
engine, err := memory.NewEngine(store, memory.Config{})
if err != nil {
	panic(err)
}
ctx := context.Background()
input := []memory.ResponseItem{{
	Type: "message", Role: "user",
	Content: []memory.ResponseContentPart{{Type: "input_text", Text: "I prefer concise answers."}},
}}
prepared, err := engine.BeforeTurn(ctx, memory.BeforeTurnInput{
	Project: "demo", SessionID: "session-001", UserID: "user-001", TurnID: "turn-001",
	ConversationItems: input, CurrentInputStart: 0, CurrentInputCount: 1,
	MaxInputTok: 8192,
})
if err != nil {
	panic(err)
}
// In the application, send prepared.InputItems to the model and collect its output.
output := []memory.ResponseItem{{
	Type: "message", Role: "assistant",
	Content: []memory.ResponseContentPart{{Type: "output_text", Text: "Understood."}},
}}
if err := engine.AfterTurn(ctx, memory.AfterTurnInput{
	Project: "demo", SessionID: "session-001", UserID: "user-001", TurnID: "turn-001",
	ConversationItems: prepared.InputItems,
	CurrentInputStart: len(prepared.InputItems) - 1, CurrentInputCount: 1,
	OutputItems: output,
}); err != nil {
	panic(err)
}
fmt.Println("turn persisted")
```

The `len(...)-1` index above is specific to this one-current-message recipe. Track the correct span for multi-item, tool, or multimodal turns rather than copying that index unchanged. Preserve the same project/session/turn identity between preparation and persistence.

`memory.Config{}` uses default behavior and rule-based fact extraction. A custom `HeuristicClient`, or configured `LLMAPIBase` plus `LLMAPIKey`, enables model-based extraction and can send turn/history data to that endpoint. Configure consent, retention, and access boundaries before enabling it. `MaxInputTok` is a compaction hint with estimated token accounting, not a hard provider-token or spending cap. Turn deduplication has a bounded remembered-ID window, not an indefinite exactly-once guarantee.

For operations, use the `memory.Management` methods `RunMaintenance`, `RunConsolidation`, and `ListDirWithAbstract`. Configure `L1RetentionDays`/`L2RetentionDays`, compaction, and maintenance policy for your application. The manual documents storage layout, compatibility inputs, defaults, and V1-to-V2 reconciliation in detail.

[Complete memory manual](manual/agents_memory.md) · [Architecture](arch/agents_memory_v2.md) · [Memory reference](ref/agents_memory.md)

## Command packages

**Core functionality:** `cmd/gutils` is the executable entry point; `cmd` exports Cobra commands for embedding in another CLI. Install and command examples are in the [README](../README.md#command-line-toolbox).

With `gcmd "github.com/Laisky/go-utils/v6/cmd"` and `"github.com/spf13/cobra"`, a host CLI can attach commands in a function body:

```go
root := &cobra.Command{Use: "my-tool"}
root.AddCommand(gcmd.EncryptCMD)
root.AddCommand(gcmd.DecryptCMD)
if err := root.Execute(); err != nil {
	panic(err)
}
```

Treat the exported Cobra command instances as mutable objects; build and execute your command tree deliberately rather than concurrently reusing them across independent invocations. Embedding does not remove the original commands' file-overwrite, secret-input, or transport requirements.

[CLI/AES guide](../cmd/README.md) · [Output-path safety](../cmd/output_safety.md)
