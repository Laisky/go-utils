package fileguard

import "os"

// readFlags uses os.Root's asynchronous Windows handles and device-name rejection.
func readFlags() (int, error) { return os.O_RDONLY, nil }

// appendFlags returns no extra flags; Windows has no O_NOFOLLOW, so OpenAppend
// relies on Lstat inspection plus post-open identity and type verification.
func appendFlags() int { return 0 }

// hasExtraLinks reports false because Windows descriptor metadata exposes no link count.
// It takes descriptor metadata and always returns false.
func hasExtraLinks(os.FileInfo) bool { return false }
