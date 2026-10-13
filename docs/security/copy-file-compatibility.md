# CopyFile overwrite compatibility

`CopyFile` retains in-place regular-file overwrite: the existing inode, mode,
ownership, hardlinks and open-handle visibility remain. Creation mode is not chmod.
It is deprecated in favor of the explicit `CopyFileAtomic` when atomic publication
and isolation from pre-existing hardlinks are required. The latter replaces the
inode and uses the requested creation mode, exactly as the hardened implementation
introduced in e30a5f7. It is not a semantic drop-in for in-place writers.

Both APIs retain exclusive creation without Overwrite, and reject final symlinks,
directories and special files. In-place opening removes truncation/create flags,
uses nonblocking/no-follow flags where supported, and validates the same regular
inode before truncating. Same-file copying is rejected without truncating source.
Destination parent directories must be trusted. In-place I/O failure cannot roll
back bytes already written; use the explicit atomic API when that guarantee matters.
No automatic inode replacement, data migration, dependency or CI-policy change.

Behavior-first regressions prove inode/mode/open-handle and hardlink updates before
and after the repair. Atomic-only tests move to the explicit replacement API;
existing symlink, directory, creation and race controls remain on CopyFile.
