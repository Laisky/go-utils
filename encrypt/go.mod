module github.com/Laisky/go-utils/v2/encrypt

go 1.18

require (
	github.com/Laisky/go-utils/v2 v2.1.0
	github.com/Laisky/go-utils/v2/log v0.0.1
	github.com/Laisky/zap v1.19.3-0.20220707055623-fe1750cd1b41
	github.com/cespare/xxhash v1.1.0
	github.com/monnand/dhkx v0.0.0-20180522003156-9e5b033f1ac4
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.0
	golang.org/x/crypto v0.0.0-20220411220226-7b82a4e95df4
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
)

require (
	github.com/Laisky/fast-skiplist v0.0.0-20210907063351-e00546c800a6 // indirect
	github.com/Laisky/go-chaining v0.0.0-20180507092046-43dcdc5a21be // indirect
	github.com/Laisky/graphql v1.0.5 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/gammazero/deque v0.1.1 // indirect
	github.com/google/go-cpy v0.0.0-20211218193943-a9c933c06932 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/automaxprocs v1.4.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/lint v0.0.0-20190930215403-16217165b5de // indirect
	golang.org/x/sys v0.0.0-20220412211240-33da011f77ad // indirect
	golang.org/x/tools v0.1.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/Laisky/go-utils/v2 v2.1.0 => ../.
	github.com/Laisky/go-utils/v2/log v0.0.1 => ../log
)
