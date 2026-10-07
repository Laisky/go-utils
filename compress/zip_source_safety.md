# ZIP source selection

`ZipFiles` and `AddFileToZip` reject source symlinks, including in-root links,
dangling links, directory links, cycles and explicitly selected final links.
Regular files and real directories are read through anchored `os.Root` handles.
Metadata and opened-object identities must agree before reading or enumeration;
a link substituted after inspection cannot redirect reads outside that root.
Unix opens include nonblocking mode so a substituted FIFO cannot hang the open.
Unsupported platforms fail closed rather than claiming race-resistant reads.

The parent paths of explicitly selected sources and the output directory are
caller-controlled trust boundaries. The caller must also control mount points
and directory moves. This does not distinguish hard links to the same inode,
defend against privileged mount changes, or provide a filesystem snapshot against
same-inode concurrent modification. Size/mtime changes invalidate the archive.
Each file copies at most its inspected size. Traversal reads directory entries in
batches of 128 and rejects more than 100,000 entries or 256 nested directories.

`ZipFiles` writes a private 0600 temporary archive, finalizes the ZIP, syncs and
closes it, and only then renames it over an authorized regular output. Input or
finalization errors preserve an existing output; staging files are removed.
Sources overlapping either the output or staging inode are rejected. Final
output links and special files are rejected. The general filesystem output-link
policy in issue #46 remains separate.

`AddFileToZip` writes to a caller-owned writer and cannot undo earlier entries.
On any error, discard that writer; on success the caller still must check the
ZIP writer's final Close error. These APIs do not sanitize file contents or make
untrusted source data safe to execute.

Rooted names are cleaned before every rooted operation; trailing separators are
not passed through. Applications should still use a security-patched Go runtime
(e.g. Go 1.25.12+ or Go 1.27.1), not rely on compatibility testing under 1.25.7.
