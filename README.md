# Go-Utils

Many useful golang tools

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Commitizen friendly](https://img.shields.io/badge/commitizen-friendly-brightgreen.svg)](http://commitizen.github.io/cz-cli/)
[![Go Report Card](https://goreportcard.com/badge/github.com/Laisky/go-utils)](https://goreportcard.com/report/github.com/Laisky/go-utils)
[![GoDoc](https://godoc.org/github.com/Laisky/go-utils?status.svg)](https://godoc.org/github.com/Laisky/go-utils)
[![Build Status](https://travis-ci.org/Laisky/go-utils.svg?branch=master)](https://travis-ci.org/Laisky/go-utils)


Install:

```sh
go get github.com/Laisky/go-utils
```

## Usage

```go
import (
    "github.com/Laisky/go-utils"
)
```


There are small tools including:

- `Clock`: high performance lazy load clock
- `Settings`: configuration manager that support yml and spring-cloud-config-server
- `Counter`: counter and rotate counter
- `Mail`: simply email sender
- `JWT`: simply JWT encrypt/decrypt functions
- `RequestJSON`: simply http client that send json request and unmarshal response by json
- `Logger`: high performance structrued logger based by zap
- `Math`: some simply math functions
  - `Round`: get round of float
- `Throttle`: throttling to limit throughput
- time: some useful time functions
  - `UTCNow()`
  - `ParseTs2String`
  - `ParseTs2Time`
- utils: some tool functions
  - `GetFuncName`
  - `FallBack`
  - `RegexNamedSubMatch`
  - `FlattenMap`


see more examples in  tests or [document](https://godoc.org/github.com/Laisky/go-utils)
