# Go-Utils


Install:

```sh
go get github.com/Laisky/go-utils
```

---

## Usage

```go
import (
    "github.com/Laisky/go-utils"
)
```

### Settings

Read config file (yaml, named `settings.yml`):

```
utils.Settings.Setup("/etc/xxx/")  // load `/etc/xxx/settings.yml`
```

Bind Pflags:

```go
func main() {
	pflag.Bool("debug", false, "run in debug mode")
	pflag.Bool("dry", false, "run in dry mode")
	pflag.String("config", "/etc/go-ramjet/settings", "config file directory path")
	pflag.StringSliceP("task", "t", []string{}, "which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
    pflag.Parse()

    // bind pflags to settings
    utils.Settings.BindPFlags(pflag.CommandLine)
}
```

Usage:

```go
utils.Settings.Set(string, interface{})
utils.Settings.Get(string) interface{}
utils.Settings.GetString(string) string
```
