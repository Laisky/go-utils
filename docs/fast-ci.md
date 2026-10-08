# CI testing policy (2026-10-08)

At the maintainer's request, general automatic pre-merge testing is formatting
plus the fastest essential unit tests. This scheduling policy supersedes the
earlier requirement to run full general lint and race/coverage qualification on
every update. Tests and assertions are unchanged. Developers remain responsible
for full local dev/staging verification before promoting relevant Go changes.

## Automatic gate

`Test / test` uses the latest Go 1.26 patch with `GOTOOLCHAIN=local`, checks every
tracked Go source with `gofmt -s -l`, and runs seven named tests from
`.scripts/fast_ci_tests.json`. These cover a SHA-256 vector, URL password masking,
slice cleanup, JSON validation/comment handling and logger field isolation/filtering.
They need no external service, Tongsuo, benchmark, fuzz or long timing campaign.
The selected packages compile normally, including their existing test files.

The allowlist must be nonempty and unique, discovery must find every name, and
every selected test must run and pass exactly once. Any skipped subtest, failure,
missing package completion, invalid JSON, native command failure or timeout fails
the gate. Test caching is disabled with `-count=1`; build caching remains enabled.
Raw discovery, Go JSON, native exits and wall times are uploaded for successful
and failed executions. The five-minute job timeout bounds this essential gate.
It does not establish full library correctness or production readiness.

```sh
python3 -m unittest discover -s .scripts -p test_fast_ci.py -v
python3 .scripts/fast_ci.py --evidence /tmp/go-utils-fast-ci
```

## Retained manual qualification

Dispatch the `Test` workflow on the candidate branch to run the retained full
lint job and Go 1.26/current-stable build and race/coverage matrix, including the
pinned Tongsuo 8.5.0 build/install and coverage upload. The essential gate also
runs on dispatch. These full jobs keep their existing timeouts and assertions.

For local dev/staging, install the tools used by `make lint` (goimports,
golangci-lint v2 and govulncheck), and install Tongsuo 8.5.0 in `PATH` so its
integration tests execute. Without Tongsuo, their existing skips are not evidence
of successful integration verification. Use a Unix dev/staging environment for
the platform-specific full suite and race detector.

```sh
make lint
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test -race -timeout 45m -coverprofile=coverage.txt -covermode=atomic ./...
# Repeat with an installed latest stable toolchain for compatibility verification.
go test -run '^$' -bench . -benchmem ./...
go test -run '^$' -fuzz '^FuzzRBACRestrictionMonotonicity$' -fuzztime=30s -parallel=2 -timeout=5m rbac*.go
```

Fuzz campaigns run one named target and package per command. Discover retained
targets with `go test -list '^Fuzz' ./...`, then substitute a target and package.
Run environment, process, stress and performance campaigns manually with the
necessary local/staging dependencies; no test files are removed.

## Security boundary

Govulncheck keeps its existing automatic push/PR triggers in
`security-scan.yml`, using the patched Go 1.26 toolchain. The separate
`crypto-tests.yml` and `rbac-security.yml` workflows are unchanged, including
their race, full-suite and fuzz checks. These security jobs can exceed the
essential gate's five-minute target; changing them requires a separately scoped
security decision. Branch protection, secrets and deployment settings are
unchanged by this scheduling amendment.
