some useful cmds.

Example:

append to your root cmd.

```go
import (
	gcmd "github.com/Laisky/go-utils/v4/cmd"
)

func init() {
	rootCmd.AddCommand(gcmd.EncryptCMD)
}

```
