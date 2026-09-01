# Repository Engineering Requirements

These instructions apply to every file in this repository. Treat them as the
minimum definition of done for every change. Preserve backward compatibility
unless the user explicitly authorizes a breaking change.

## Required workflow

Before editing code:

1. Inspect the production call path affected by the change.
2. Record a reproducible benchmark baseline for that path.
3. Identify which Go and C code owns every allocation, handle, pointer, and
   connection touched by the change.

After editing code:

1. Run the same benchmark with identical machine, SDK, environment,
   `GOMAXPROCS`, `-benchtime`, and `-count` settings.
2. Run the correctness and memory-safety checks below.
3. Report the exact commands, before/after data, variance or sample count, and
   any checks that could not run. Never describe an optimization as successful
   when the difference is within benchmark noise.

Documentation-only changes may mark benchmark and memory-safety checks as not
applicable. Test-only changes must still be compiled and run.

## Benchmark requirements

Every production-code change must have a benchmark that exercises the changed
production path. Add or extend a stable Go benchmark when none exists. Do not
benchmark a rewritten copy of the implementation.

Use multiple samples. The default local command is:

```bash
go test ./gorfc -run '^$' -bench '<affected benchmark>' -benchmem -benchtime 1s -count 8
```

Keep raw before and after output so it can be compared with `benchstat` when
available. At minimum report `ns/op`, `B/op`, and `allocs/op`. For concurrent
changes also report throughput and test more than one relevant `GOMAXPROCS`
value. Reject unexplained statistically meaningful regressions. Do not trade
correctness, memory safety, or compatibility for benchmark improvements.

For changes affecting `Connection.Call`, network behavior, or
`ConnectionPool`, also run the opt-in SAP benchmark from
`gorfc/production_benchmark_test.go` against an explicitly authorized staging
destination:

```bash
GORFC_BENCH_DEST=QAS go test ./gorfc -run '^$' -bench BenchmarkProduction -benchmem -benchtime 10s -count 5
```

Never infer or invent a production destination. Never run load benchmarks
against production without explicit user authorization. If no SAP destination
is available, state that limitation and do not substitute assumptions for
end-to-end data.

## Mandatory correctness and memory-safety checks

Run all checks supported by the current platform:

```bash
go test ./... -count 1
go test -race ./... -count 1
go test ./... -gcflags=all=-d=checkptr=2 -count 1
go vet ./...
```

Tests requiring an external SAP system may be isolated only when that system is
unavailable. In that case, run all local tests plus a compile-only pass with
`go test ./... -run '^$'`, and clearly report which integration tests were not
run and why.

When changing C allocation, pointer conversion, or cleanup code, add focused
tests covering success, empty input, maximum practical size, malformed input,
and every reachable error path. Use sanitizer or leak-checking builds when the
local compiler and SAP SDK support them. A skipped or unavailable sanitizer is
not evidence of memory safety; pair it with code-path ownership analysis and
the Go race/checkptr checks.

## CGO ownership rules

- Document ownership at the allocation or transfer site when it is not
  obvious.
- Every `C.malloc`, `C.GoMallocU`, SDK create/open call, or equivalent resource
  acquisition must have exactly one matching cleanup on every exit path.
- Go-allocated memory is owned by Go. C must not retain a Go pointer after the
  CGO call returns. Follow the CGO pointer rules and use `runtime.KeepAlive`
  where lifetime is otherwise ambiguous.
- C-allocated memory is owned by C until explicitly transferred. Release it
  with the matching C/SDK function, never a Go allocator.
- Do not construct Go slices or strings that outlive their backing C memory.
  Copy data into Go-owned memory before freeing the C allocation.
- Set handles/pointers to `nil` after successful release when reuse or repeated
  cleanup is possible.
- Finalizers are fallback protection, not primary ownership. Deterministic
  `Close`, `Destroy`, `Release`, or `free` paths remain mandatory.
- Pair `RfcCreateFunction` with `RfcDestroyFunction`, open connections with
  close, checked-out pooled connections with `Release`, and native buffers with
  the allocator's matching free operation.
- Never hold a Go mutex across a blocking SAP call unless serialization is an
  intentional part of the public contract and is covered by contention and
  deadlock tests.
- Check nil, zero-length, overflow, and conversion bounds before pointer
  arithmetic or converting lengths to C integer types.

## Compatibility requirements

- Do not remove or rename exported identifiers, change method signatures,
  narrow accepted inputs, alter returned Go types, or change default connection
  semantics without explicit authorization.
- New performance features must be opt-in unless they are behaviorally
  equivalent and validated by existing tests.
- Preserve stateful connection behavior and SAP SDK error information.
- Add regression tests for every bug fix and boundary test for every changed
  buffer-size calculation.

## Required final report

Every completed code change must report:

- production path affected;
- benchmark command and before/after results;
- correctness, race, checkptr, vet, and integration-test status;
- ownership and cleanup paths reviewed;
- compatibility impact;
- limitations, skipped checks, and the reason for each skip.

Do not claim that a change is production-proven when only a microbenchmark was
run. Distinguish local/native SDK results from end-to-end SAP measurements.
