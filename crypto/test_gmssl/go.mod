module test_gmssl

go 1.26.0

require (
	github.com/GmSSL/GmSSL-Go v1.3.1
	github.com/Laisky/go-utils/v6 v6.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.1
	// v1.0.0 includes <openssl/sm4.h>, which Tongsuo installs only when built with enable-export-sm4.
	github.com/tongsuo-project/tongsuo-go-sdk v0.0.0-20231225081335-82a881b9b3d3
)

require (
	github.com/Laisky/errors/v2 v2.0.1 // indirect
	github.com/Laisky/fast-skiplist/v2 v2.0.1 // indirect
	github.com/Laisky/go-chaining v0.0.0-20180507092046-43dcdc5a21be // indirect
	github.com/Laisky/golang-fifo v1.0.1-0.20240403092208-1d90c6c33e11 // indirect
	github.com/Laisky/graphql v1.0.6 // indirect
	github.com/Laisky/zap v1.27.1-0.20261006114731-55f41c2b5061 // indirect
	github.com/alexvec/go-bip39 v1.1.0 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/emmansun/gmsm v0.45.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gammazero/deque v1.2.1 // indirect
	github.com/google/go-cpy v0.0.0-20211218193943-a9c933c06932 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monnand/dhkx v0.0.0-20180522003156-9e5b033f1ac4 // indirect
	github.com/tailscale/hujson v0.0.0-20260302212456-ecc657c15afd // indirect
	github.com/xlzd/gotp v0.1.0 // indirect
	go.dedis.ch/kyber/v3 v3.1.0 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/tools v0.1.5 // indirect
)

// Always test the go-utils working tree that contains this module.
replace github.com/Laisky/go-utils/v6 => ../..
