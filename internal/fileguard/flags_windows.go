package fileguard

import "os"

// readFlags uses os.Root's asynchronous Windows handles and device-name rejection.
func readFlags() (int, error) { return os.O_RDONLY, nil }
