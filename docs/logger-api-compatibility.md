# Legacy v1 logger contracts

The v1.17 interface-returning factories, global default and fluent signatures remain unchanged. This maintenance change adds no new public logger API and does not reverse a published contract.

The proposed parallel `Concrete` factory/method family was withdrawn before release: it duplicated the public API to work around one legacy dependency graph. Current logging integrations should use the existing typed `github.com/Laisky/go-utils/v6/log` API, after testing the real consumer graph and its Go requirements.

`v6/log.New` returns `*log.LoggerT`; `Named`, `With` and `WithOptions` preserve that concrete type. Consumers should define the small logging interfaces they actually use instead of requiring this package's entire historical interface or concrete wrapper.

Migrating from v1 to v6 is explicit, not a drop-in promise: import paths, logger/global names, level types and other utility APIs can differ. Legacy v1 tags remain immutable. No API version, tag or package is published by this source change.

Mixing the current v6 module with the old v1 root utility dependency graph also exposes a separate `gammazero/deque` generic-API conflict. The logging shape is coherent, but that whole mixed graph has not passed qualification; a real v6 migration must resolve the utility graph rather than add a logger cast.

## Compiler requirement

This maintenance candidate requires Go 1.18 or newer because security-fixed JWT v4.5.2 uses strings.Cut. Its dependency metadata still says Go 1.16; actual Go 1.16.15 and 1.17.13 builds fail, while the previous SDK base builds on both. This is an explicit compiler-support change, not a logger API signature change. Keep the patched dependency rather than a vulnerable downgrade, private fork or parser bypass.

Go 1.18.10 is the minimum-line test compiler and Go 1.27.3 is the current race/regression compiler. Historical Go 1.13-1.17 matrix entries are not ongoing compatibility promises. Existing published tags are unchanged. A release must communicate this requirement; no tag or release is created by the source change.
