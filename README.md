# Go-Utils

Many useful golang tools, support >= v1.13.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Commitizen friendly](https://img.shields.io/badge/commitizen-friendly-brightgreen.svg)](http://commitizen.github.io/cz-cli/)
[![Go Report Card](https://goreportcard.com/badge/github.com/Laisky/go-utils)](https://goreportcard.com/report/github.com/Laisky/go-utils)
[![GoDoc](https://godoc.org/github.com/Laisky/go-utils?status.svg)](https://pkg.go.dev/github.com/Laisky/go-utils?tab=doc)
![Build Status](https://github.com/Laisky/go-utils/actions/workflows/test.yml/badge.svg?branch=master)
[![codecov](https://codecov.io/gh/Laisky/go-utils/branch/master/graph/badge.svg)](https://codecov.io/gh/Laisky/go-utils)

Install:

```sh
go get github.com/Laisky/go-utils
```

## Usage

```go
import (
    gutils "github.com/Laisky/go-utils/v2"
)
```


## Modules

Contains some useful tools in different directories:

* `color.go`: colorful code
* `compressor.go`: compress and extract dir/files
* `configserver.go`: load configs from file or config-server
* `email.go`: SMTP email sdk
* `encrypt.go`: some tools for encrypt and decrypt,
                support AES, RSA, ECDSA, MD5, SHA128, SHA256
* `fs.go`: some tools to read, move, walk dir/files
* `http.go`: some tools to send http request
* `jwt.go`: some tools to generate and parse JWT
* `logger.go`: enhanched zap logger
* `math.go`: some math tools to deal with int, round
* `net.go`: some tools to deal with tcp/udp
* `random.go`: generate random string, int
* `settings.go`: read configs from file or config-server
* `sort.go`: easier to sort
* `sync.go`: some locks depends on atomic
* `throttle.go`: faster rate limiter
* `time.go`: faster clock (if you do not enable vdso)
* `utils`: some useful tools


# Thanks

Thanks to JetBrain support OpenSource License.

<https://www.jetbrains.com/community/opensource/#support>
