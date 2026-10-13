# Directory AES input compatibility

A filename suffix is not evidence of ciphertext. Existing directory-encryption
APIs include matching regular plaintext files such as `report.enc`, writing
`report.enc.enc`, just as the historical extension filter specified. The new
`WithAESFilesInDirSkipSuffixInputs` option explicitly requests suffix filtering;
it should only be used when the caller's naming policy establishes that meaning.

Inputs are enumerated once within existing count/byte budgets. When an output
name is also an original input (a, a.enc, a.enc.enc), those bounded inputs are
processed in descending name length before predecessors can replace them.
Non-overlapping runs retain the bounded worker pool. Created outputs never enter
the same run's input snapshot. This avoids worker-order-dependent source loss.

Existing regular output replacement remains supported; symlinks, special files,
source substitution, stale staging files, cancellation and read/growth budgets
keep their defenses. No file format or function signature changes. Repeated
unfiltered runs intentionally include their newly existing inputs; applications
that require naming-policy-based idempotence can opt into suffix filtering.

The `encrypt aes` CLI follows the same rule in both raw-key and password modes:
ordinary `.enc` inputs are included by default. `--skip-suffix-inputs` is the
explicit directory-only filename filter. It is not ciphertext detection and can
skip plaintext by design only when requested. Runs use an input snapshot, so
outputs newly produced during a run are never recursively fed back into it.
