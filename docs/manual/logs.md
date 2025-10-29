# Logging Guide

This guide walks through the `log` package provided by `go-utils`, covering setup, runtime controls, sampling, file rotation, and retention.

## Quick Start

Use the shared logger or create your own instance:

```go
package main

import (
    "github.com/Laisky/go-utils/v5/log"
    "github.com/Laisky/zap/zapcore"
)

func main() {
    logger, err := log.New(
        log.WithName("service"),
        log.WithEncoding(log.EncodingJSON),
        log.WithLevel(log.LevelInfo),
    )
    if err != nil {
        panic(err)
    }

    logger.Info("service started", zapcore.Field{Key: "version", Type: zapcore.StringType, String: "1.0.0"})
}
```

Or rely on the global `log.Shared` instance. Its level defaults to `info` and can be overridden through the `GUTILS_LOGGER_LEVEL` environment variable.

## Runtime Level Control

Levels are captured by the `Level` enum (`debug`, `info`, `warn`, `error`, `fatal`, and `panic`). Retrieve or change a logger’s level without rebuilding it:

```go
child := logger.Named("worker")
child.Info("starting")

if err := child.ChangeLevel(log.LevelDebug); err != nil {
    panic(err)
}
child.Debug("worker ready")
```

A change propagates to parents and children because every logger created through this package shares the same `zap.AtomicLevel` handle.

## Structured Context and Sampling

Create scoped loggers and attach structured fields:

```go
reqLogger := logger.With(zapcore.Field{Key: "request_id", Type: zapcore.StringType, String: id})
reqLogger.Warn("validation failed")
```

Use sampling helpers to reduce chatter while retaining occasional visibility:

```go
logger.DebugSample(10, "heartbeat")   // 1% chance
logger.InfoSample(100, "stats")       // 10% chance
logger.WarnSample(1000, "always on")  // 100% chance
```

## File Rotation

Configure time-based rotation with one of the built-in intervals:

- `log.RotationHourly`
- `log.RotationDaily`
- `log.RotationWeekly`

Rotations occur at the top of the interval **in UTC** (hourly at `HH:00`, daily at `00:00`, weekly at `Monday 00:00`).

```go
logger, err := log.New(
    log.WithEncoding(log.EncodingJSON),
    log.WithRotation("/var/log/app/service.log", log.RotationDaily),
)
if err != nil {
    panic(err)
}
```

Each window writes to a date-stamped file named `{logger}-YYYYMMDD.log` by default. For example, with a logger named `service`, the daily rotation creates `service-20251028.log`. Hourly rotations automatically append the window start time (for example `service-20251028-150000.log`) to guarantee unique filenames. Customize the pattern with `log.WithRotationFilenamePattern`, which accepts the tokens `{logger}`, `YYYY`, `MM`, `DD`, `hh`/`HH`, `mm`, and `ss`:

```go
logger, err := log.New(
    log.WithRotation("/var/log/app/service.log", log.RotationDaily),
    log.WithRotationFilenamePattern("{logger}-YYYYMMDD-HH.log"),
)
```

## Retention Policy

Control how long rotated files are retained with `WithRotationRetention`:

```go
logger, err := log.New(
    log.WithRotation("/var/log/app/service.log", log.RotationDaily),
    log.WithRotationRetention(7),
)
```

- `WithRotationRetention(0)` (default) keeps all historical rotations.
- Positive values keep rotations whose start time falls within the most recent `n` days, evaluated in UTC.
- The active log file is never removed, ensuring that configuring a retention window will not delete the file currently being written.

Retention is evaluated immediately after each rotation and also when the writer is first created.

## Flushing

Call `logger.Sync()` during shutdown to flush buffers and close files. The rotation writer also exposes `Sync` and `Close` to support graceful cleanup when the application manages the sink directly.

## Summary

The `log` package layers friendly helpers on top of `zap`, enabling:

- Hierarchical, structured logging with runtime level changes.
- Probabilistic sampling for low-signal events.
- UTC-aligned rotation with hourly, daily, or weekly cadences.
- Automatic cleanup of rotated files based on a configurable retention window.

Adopt these building blocks to keep your application logs structured, succinct, and easy to manage in production environments.
