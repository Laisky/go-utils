module github.com/Laisky/go-utils/v2/cmd

go 1.18

require (
	github.com/Laisky/go-utils/v2 v2.1.0
	github.com/Laisky/go-utils/v2/config v0.0.1
	github.com/Laisky/go-utils/v2/log v0.0.1
	github.com/Laisky/zap v1.19.3-0.20220707055623-fe1750cd1b41
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.4.0
)

require (
	github.com/Laisky/fast-skiplist v0.0.0-20210907063351-e00546c800a6 // indirect
	github.com/Laisky/go-chaining v0.0.0-20180507092046-43dcdc5a21be // indirect
	github.com/Laisky/go-utils/v2/encrypt v0.0.1 // indirect
	github.com/Laisky/graphql v1.0.5 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/gammazero/deque v0.1.1 // indirect
	github.com/google/go-cpy v0.0.0-20211218193943-a9c933c06932 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monnand/dhkx v0.0.0-20180522003156-9e5b033f1ac4 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.0.1 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.12.0 // indirect
	github.com/subosito/gotenv v1.3.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/automaxprocs v1.4.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/crypto v0.0.0-20220411220226-7b82a4e95df4 // indirect
	golang.org/x/lint v0.0.0-20201208152925-83fdc39ff7b5 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.5 // indirect
	gopkg.in/ini.v1 v1.66.4 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/Laisky/go-utils/v2 v2.1.0 => ../.
	github.com/Laisky/go-utils/v2/config v0.0.1 => ../config
	github.com/Laisky/go-utils/v2/encrypt v0.0.1 => ../encrypt
	github.com/Laisky/go-utils/v2/log v0.0.1 => ../log
)
