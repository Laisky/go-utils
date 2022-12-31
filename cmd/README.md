some useful cmds.


## CommandLine Tool

```sh
go install github.com/Laisky/go-utils/v3/cmd/gutils@latest
```

## SDK

append to your root cmd.

```go
import (
	gcmd "github.com/Laisky/go-utils/v3/cmd"
)

func init() {
	rootCmd.AddCommand(gcmd.EncryptCMD)
}
```
