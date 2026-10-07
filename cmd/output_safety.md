# gutils output and destructive-operation safety

## Output files never follow links (issue #46)

| Command | Output | Behavior |
| --- | --- | --- |
| `json sort` | rewrites each JSON file | Only regular files are rewritten. An explicit path that is a link or special file is rejected; directory scans skip such entries. The file is read through a descriptor bound to the inspected inode and the sorted result is published through a temporary file and `rename`, keeping the original permission bits. The rewritten file is a new inode owned by the current user. |
| `image favicon` | `favicon.ico` / `favicon.png` | Without `--force` the output is created exclusively, so any existing entry, including a dangling link, is refused. With `--force` an existing regular file is replaced through a temporary file and `rename`; links and special files are rejected. |
| `rsa sign` | `<file>.sig` | Re-signing replaces an existing regular `.sig` through a temporary file and `rename`; a link or special file at that path is rejected and its target is never written. |

The output directory itself must be trusted; only the final path component is
protected.
