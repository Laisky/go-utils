# File hash resource and source policy

`FileHash`, `VerifyFileHash` and the compatibility alias `ValidateFileHash` now
accept regular files only, reject final symlinks and special files, and enforce
an inclusive 1 GiB default input limit. The selected file's parent must be trusted.
A rooted open plus descriptor identity check prevents replacing an inspected
regular file with a different inode. On Unix, nonblocking open prevents a raced
FIFO from blocking before the descriptor can be checked.

`FileHashWithContext` and `VerifyFileHashWithContext` accept explicit finite byte
limits. Zero chooses the default and negative limits fail. Limits are checked
before reading and throughout the read, including a single-byte EOF probe; input
growth cannot create unbounded work. Size/mtime changes reject the result rather
than declaring a stable-file verification. Same-inode modifications that preserve
metadata require filesystem snapshot/locking controls outside these helpers.

Cancellation checks happen before I/O and between bounded chunks and close the
opened descriptor. The close callback is joined before return; no detached read
worker hides a blocked syscall. **This is not a hard wall-clock guarantee for an
uninterruptible filesystem syscall**, such as a hung network filesystem. Use a
reapable worker process or filesystem-level controls when a hard deadline is
required. Ordinary FIFO/device inputs are rejected rather than read in that mode.

`Hash` and `HashVerify` retain their existing reader-owned streaming contract;
callers deliberately processing streams must supply their own lifecycle and
resource controls. Large trusted files require an explicit larger file budget.
MD5 verification is retained only for compatibility, not collision resistance.
Digest parsing is bounded before file access; verification uses a constant-time
comparison and diagnostics do not include the expected or actual digest.
