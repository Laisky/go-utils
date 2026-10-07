# AES safety notes

This note describes the AES formats and helpers in `crypto`. It covers which API to use, the wire formats, the
resource limits, and how to migrate from the removed or unsafe paths.

## Choosing an API

| Need                                         | Use                                                            | Integrity |
| -------------------------------------------- | -------------------------------------------------------------- | --------- |
| Message in memory, random 16/24/32-byte key  | `AEADEncrypt` / `AEADDecrypt` (AES-GCM)                         | yes       |
| Large stream, random 16/24/32-byte key       | `AEADStreamEncrypt` / `AEADStreamDecrypt` (chunked AES-256-GCM) | yes       |
| Human password                               | `EncryptByPassword` / `DecryptByPassword`, `PasswordEncryptor`  | yes       |
| Low-level CTR with your own MAC              | `AesCtrStreamEncrypt` / `AesCtrStreamDecrypt`                   | **no**    |

`AesReaderWrapper` and `NewAesReaderWrapper` are deprecated. They read and authenticate the whole GCM message.
Migrate to `AEADDecrypt`, or to `AEADStreamDecrypt` for new streaming data. **Do not migrate to
`AesCtrStreamDecrypt`.** CTR has no tag: a flipped ciphertext bit flips the same plaintext bit, and the API reports
no error.

## Raw CTR contract

`AesCtrStreamEncrypt` returns `IV(16) || CTR(plaintext)`. It has no tag, length or end marker. You may use it only
if you authenticate the whole serialized stream, IV included, with an independent mechanism. For example, use
HMAC-SHA256 under a separately derived key. Verify that MAC before you trust or act on any decrypted byte.

## Authenticated stream format (v1)

```
header  = "GUSTREAM" (8) || version 0x01 (1) || chunk size uint32 BE (4) || salt (32)
subkey  = HKDF-SHA256(ikm = key, salt = salt, info = label || header)   -> 32 bytes, AES-256-GCM
chunk i = AES-256-GCM(subkey, nonce_i, plaintext_i)                      -> len(plaintext_i) + 16
nonce_i = 11-byte big-endian i || flag (0x01 for the final chunk, else 0x00)
```

- The input key may be 16, 24 or 32 bytes. The derived subkey is always 32 bytes, and every stream has a fresh
  random salt.
- Chunks are 64 KiB by default (`WithAEADStreamChunkSize`). The accepted sizes are 64 B to 4 MiB. The decoder
  checks the chunk size in the untrusted header against these bounds before it allocates buffers.
- Every chunk except the last holds exactly the chunk size. Only the final chunk carries the final flag. Empty
  plaintext encodes as a single empty final chunk.
- The decoder rejects the following:
  - bit flips in the header, ciphertext or tag;
  - a wrong key;
  - reordered, duplicated or dropped chunks;
  - truncation at a chunk boundary or inside a chunk, including a stream with no final chunk;
  - trailing data after the final chunk;
  - an empty final chunk after data;
  - chunk-counter overflow.
- Each chunk is authenticated before its plaintext is released. Earlier chunks can reach you before the end of the
  stream is checked. **Trust the output only after `Read` returns `io.EOF` without an error first.** On any error,
  discard everything you have read.

## Password format (v1)

```
header     = "GUPWAEAD" (8) || version 0x01 (1) || KDF id 0x01 = Argon2id (1)
             || time uint32 BE (4) || memory KiB uint32 BE (4) || parallelism (1) || salt (16)
key        = Argon2id(password, salt, time, memory, parallelism) -> 32 bytes
ciphertext = header || nonce (12) || AES-256-GCM(key, nonce, plaintext, AAD = header)
```

- The default parameters follow the second recommended option in RFC 9106: t=3, m=64 MiB, p=4.
  `WithPasswordKDFParams` can change them. Encryption rejects parameters below t=2 or m=19 MiB (the OWASP
  baseline).
- Decryption checks the header before any Argon2id work. It requires 1 <= t <= 10, 8*p KiB <= m <= 256 MiB and
  1 <= p <= 16. It rejects an unknown version or KDF id. A wrong password, or any change to the header, nonce,
  ciphertext or tag, fails authentication.
- `NewPasswordEncryptor` derives the key once with a fresh salt. Each `Encrypt` call uses a fresh random nonce and
  writes a self-contained ciphertext. One encryptor accepts at most 2^32 messages (the NIST random-nonce bound).
- Password bytes are never used directly as an AES key. `AEADEncrypt` stays the explicit raw-key interface.

## Directory encryption limits

`AESEncryptFilesInDirWithContext` (raw key, `AesEncrypt` format) and `PasswordEncryptFilesInDirWithContext`
(password format) work only on the regular files directly inside the directory. They have these defaults:

| Option                               | Default             |
| ------------------------------------ | ------------------- |
| `WithAESFilesInDirMaxConcurrency`    | min(4, GOMAXPROCS)  |
| `WithAESFilesInDirMaxFiles`          | 10000               |
| `WithAESFilesInDirMaxFileBytes`      | 64 MiB              |
| `WithAESFilesInDirMaxTotalBytes`     | 1 GiB               |

- The limits are checked in three places:
  - while the directory is listed, so an over-budget run writes nothing;
  - on the opened file descriptor;
  - during a bounded read, which catches files that grow mid-run.

  Listing reads entries in batches and stops as soon as the file-count limit is exceeded.
- These entries are skipped: symlinks, FIFOs, directories and other non-regular entries; names that already end
  with the suffix; and the function's own staging files. Sources are opened without following links or blocking
  on FIFOs.
- Each output is written to a private, exclusively created temporary file in the same directory, then renamed over
  `<name><suffix>`. An existing regular output is replaced. An existing symlink (live or dangling) or special file
  is never followed or replaced, and the run fails.
- On the first error or on cancellation, no new files are started and staging files are removed. Outputs that were
  already published are kept.
- `AESEncryptFilesInDir` keeps its signature and uses `context.Background()`. The output format is still the
  authenticated `AesEncrypt` (AES-GCM) format.

## CLI (`encrypt aes` / `decrypt aes`)

- Secrets are never taken from argv. Use one of these inputs:
  - `--password-file <path|->` for password mode. Trailing CR/LF are removed.
  - `--key-file <path|->` for raw-key mode. The file holds only hex digits for 16, 24 or 32 bytes; surrounding
    whitespace is ignored.
  - The no-echo terminal prompt. Encryption asks for the password twice.

  Without one of these inputs, the command fails. On Unix, a secret file that group or others can access is
  rejected. Secrets are never logged or echoed in errors.
- `-s/--secret` is still registered so old scripts get a migration error, but it always fails.
- Outputs have mode 0600 and are published by temporary file and rename. An existing symlink, directory or special
  file at the output path is refused and left untouched.

### Legacy migration

Files written by the removed `encrypt aes -s <password>` used the 16, 24 or 32 password bytes directly as the
AES-GCM key. Decrypt them only with the explicit legacy flag, then encrypt them again in password mode:

```sh
go-utils decrypt aes -i old.txt.enc -o old.txt --password-file ./pw.txt --legacy-password-as-key
go-utils encrypt aes -i old.txt --password-file ./pw.txt
```
