# CopyFile overwrite contracts

`CopyFile(..., Overwrite())` retains the destination inode. Its mode, ownership,
other hardlinks and open handles continue to refer to the updated file, as before
the atomic-replacement change. `WithFileMode` is a creation mode and does not
change an existing inode. Hardlink visibility is intentional for this deprecated
compatibility API, not a claim of isolation from other names of that inode.

`CopyFileAtomic` explicitly opts into private temporary-file publication and
rename. Existing hardlinks/open handles retain old content; the replacement gets
the requested creation mode and creating user's metadata. Existing atomic
hardlink-isolation tests target this new API rather than silently changing old
callers. In-place writes cannot promise rollback on IO failure; callers needing
unchanged destinations on failed writes should migrate to the atomic API.

Both APIs retain exclusive new-file creation, reject final symlinks/FIFOs/devices,
and require trusted parent directories. In-place opens remove creation/truncation
flags, validate descriptor type and identity, and only then truncate. Unix adds
O_NOFOLLOW/O_NONBLOCK. Self-copy through the same inode is rejected before any
truncation. No symlink-following or unchecked-truncation vulnerability is restored.

The behavior tests first fail on inode and hardlink visibility, then pass after
the repair. Atomic isolation and raced symlink rejection remain tested separately.
No data migration or deployment is included; choose the API based on required
filesystem semantics rather than upgrading into different behavior by accident.
