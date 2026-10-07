# gutils output and destructive-operation safety

## Output files never follow links (issue #46)

| Command | Output | Behavior |
| --- | --- | --- |
| `json sort` | rewrites each JSON file | Only regular files are rewritten. An explicit path that is a link or special file is rejected; directory scans skip such entries. The file is read through a descriptor bound to the inspected inode and the sorted result is published through a temporary file and `rename`, keeping the original permission bits. The rewritten file is a new inode owned by the current user. |
| `image favicon` | `favicon.ico` / `favicon.png` | Without `--force` the output is created exclusively, so any existing entry, including a dangling link, is refused. With `--force` an existing regular file is replaced through a temporary file and `rename`; links and special files are rejected. |
| `rsa sign` | `<file>.sig` | Re-signing replaces an existing regular `.sig` through a temporary file and `rename`; a link or special file at that path is rejected and its target is never written. |

The output directory itself must be trusted; only the final path component is
protected.

## Digest matches are only candidates (issue #52)

- `remove-dup` uses SHA-1 to find candidate duplicates, then compares file sizes
  and all bytes (streamed in 32 KiB chunks) before deleting. Different content,
  a comparison or read failure, or a file that changed after comparison keeps
  both files and logs a warning. Two names of the same inode are not treated as
  removable duplicates. `--dry` reports proven duplicates without deleting.
- `md5dir` never replaces an existing destination. Moves publish with a
  no-replace hard link and then remove the source (falling back to an exclusive
  copy when hard links are unavailable, for example across devices); `--remain`
  copies with exclusive creation. When the destination exists, identical bytes
  are deduplicated (a moved source is removed, a copy is skipped); different
  bytes keep both files, log a warning and make the command exit with an error
  after processing all other files. A file already stored at its own content
  address is left alone.

Similar-image removal in `remove-dup` is a separate, intentionally fuzzy policy
and is not covered by the byte-equality guarantee.
