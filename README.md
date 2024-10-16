# Go-Utils

Many useful golang tools

| Version | Support Go |
| ------- | ---------- |
| v1      | >= v1.16   |
| v2      | >= v1.18   |
| v3      | >= v1.19   |
| v4      | >= v1.21   |
| v5      | >= v1.23   |

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Commitizen friendly](https://img.shields.io/badge/commitizen-friendly-brightgreen.svg)](http://commitizen.github.io/cz-cli/)
[![Go Report Card](https://goreportcard.com/badge/github.com/Laisky/go-utils/v4)](https://goreportcard.com/report/github.com/Laisky/go-utils/v4)
[![GoDoc](https://godoc.org/github.com/Laisky/go-utils/v4?status.svg)](https://pkg.go.dev/github.com/Laisky/go-utils/v4)
![Build Status](https://github.com/Laisky/go-utils/actions/workflows/test.yml/badge.svg?branch=v3)
[![codecov](https://codecov.io/gh/Laisky/go-utils/branch/v3/graph/badge.svg)](https://codecov.io/gh/Laisky/go-utils)

## Use as command-line executable binary

### Install

```sh
go install github.com/Laisky/go-utils/v4/cmd/gutils@latest
```

Add `$HOME/.go/bin` to your PATH.

### Usage

```sh
# find and delete duplicate files/ similar images
gutils remove-dup examples/images --dry

# move files to hash-based hierach directories
gutils md5dir -i examples/md5dir/ --dry

# show x509 certificate details
gutils certinfo -r blog.laisky.com:443
gutils certinfo -f ./cert.pem

# encrypt by aes
gutils encrypt aes -i <file_path> -s <password>

# sign or verify by rsa
gutils rsa sign
gutils rsa verify
```

## Use as SDK

### Install

```sh
go get github.com/Laisky/go-utils/v4@latest
```

### Usage

```go
import (
    gutils "github.com/Laisky/go-utils/v4"
)
```

### Modules

Contains some useful tools in different directories:

- `settings`: move go [github.com/Laisky/go-config](https://github.com/Laisky/go-config)
- `color.go`: colorful code
- `compressor.go`: compress and extract dir/files
- `email/`: SMTP email sdk
- `encrypt/`: some tools for encrypt and decrypt,
  support AES, RSA, ECDSA, MD5, SHA128, SHA256
  - `configserver.go`: load configs from file or config-server
- `fs.go`: some tools to read, move, walk dir/files
- `http.go`: some tools to send http request
- `jwt/`: some tools to generate and parse JWT
- `log/`: enhanched zap logger
- `math.go`: some math tools to deal with int, round
- `net.go`: some tools to deal with tcp/udp
- `random.go`: generate random string, int
- `sort.go`: easier to sort
- `sync.go`: some locks depends on atomic
- `throttle.go`: faster rate limiter
- `time.go`: faster clock (if you do not enable vdso)
- `utils`: some useful tools
