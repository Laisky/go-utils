# Command output and lifecycle

`RunCMD` and `RunCMDWithEnv` now retain at most 1 MiB of combined stdout and
stderr. `RunCMD2` enforces the same total output budget plus 64 KiB per line.
Larger trusted jobs must select finite limits through `CMDOptions` and
`RunCMDWithOptions` / `RunCMD2WithOptions`. Zero selects defaults; negative
limits are rejected. An empty environment inherits the parent; a nonempty
list replaces it, as before.

`ErrCMDOutputLimit` or `ErrCMDLineLimit` cancels the direct child. `os/exec`
owns both copy loops, waits for them, and reaps the child before return. The
first output error takes precedence over the resulting kill error. Normal
CRLF and final unterminated lines are retained. Callbacks for the two streams
may run concurrently, but none is left running after return.

Callbacks must return promptly. Go cannot forcibly terminate a callback that
blocks forever. The default two-second `WaitDelay` bounds inherited-pipe cleanup
after process exit or cancellation; it is not a command runtime deadline and
cannot interrupt arbitrary callback code. Use a context deadline for execution
and OS process-group/job/container isolation when descendant termination is
required. These helpers kill/reap the direct child, not its entire process tree.

Execution errors do not embed command arguments or captured output. Returned
capture bytes and explicitly delivered callback strings remain application data
and can contain secrets. Nil streaming handlers preserve the old debug/error
logging behavior, so sensitive jobs must install explicit handlers on both
streams. Executable lookup errors can still identify the executable path.
