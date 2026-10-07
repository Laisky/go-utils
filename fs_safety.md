# File destination policy

The root-package file helpers never write through a destination link. This
closes link-following writes (issue #46) when a lower-trust local user can
pre-create entries in a directory processed by your service.

## `IsDirWritable`

The probe is created exclusively with an unpredictable name
(`.writable-probe-*`), closed and then removed. Existing entries, including a
file or link named `.touch`, are never opened, truncated or removed. An empty
directory argument means the current working directory.

## `CopyFile` and `MoveFile`

- **Default (no `Overwrite`)**: the destination is created atomically with
  `O_CREATE|O_EXCL`. Any existing entry is a collision, including a symbolic
  link whose target does not exist; the error matches `fs.ErrExist`. There is no
  separate existence check that could race with the open.
- **`Overwrite()`**: an existing destination must be a regular file. Symbolic
  links, directories, FIFOs and devices are rejected and left unchanged. The new
  content is written to a private, exclusively created temporary file in the
  destination directory and published with `rename`. Rename replaces the
  directory entry, so a link target or another hard link of the previous file is
  never modified.
- A failed copy never leaves a partial destination: the exclusively created file
  or the temporary file is removed.
- `MoveFile` uses the default exclusive mode, so `src` is kept when `dst`
  already exists or is a dangling link.

### Migration notes

- `Overwrite()` now replaces the destination inode. The replacement file is
  created with the `WithFileMode` mode (default `0640`, reduced by the umask);
  the previous file's mode, owner and extra hard links are not preserved. Pass
  `WithFileMode` explicitly when a specific mode is required.
- `WithFileFlag` still adds flags such as `O_SYNC` or `O_APPEND` to the newly
  created file. `O_TRUNC` is ignored because an existing file is never opened
  for writing.
- Code that relied on `Overwrite()` writing through a symbolic link must now
  resolve the link explicitly and pass the real path, or replace the link.

## Trust boundary

These helpers protect the final path component. Parent directories of the
destination are resolved normally and must be trusted by the caller (owned by
the service, not writable by lower-trust users). `compress.Unzip` uses rooted
directory handles for archive member paths; see `compress/unzip_safety.md`.
