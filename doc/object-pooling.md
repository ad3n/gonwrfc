# Input conversion object pooling

`fillStringWithLength` now borrows a pointer-free `inputConversionState` from
`sync.Pool` for the SDK error structure and conversion lengths. This path is
used for function/parameter names and string inputs, including `Connection.Call`.
Inputs above 32 KiB use the original unpooled converter; benchmarking the
unbounded pool revealed a regression for large ASCII inputs. The existing
output buffer pool and connection pool are unchanged.

## Ownership and compatibility

Go owns the scratch object. The synchronous `RfcUTF8ToSAPUC` call borrows its
fields and retains no Go pointers. The SAP SDK header declares the length and
error arguments as output/in-out arguments. The scratch object contains only
C scalar values and fixed arrays; it never contains a Go or native pointer.
CGO keeps arguments alive for the call. A deferred cleanup zeroes the entire
object and returns it on success and error. The returned length is copied,
and `rfcError` converts SDK fields into independently owned Go strings before
cleanup. Pool eviction by Go's GC is safe and needs no native cleanup.

The SAP_UC result still comes from `GoMallocU`/`mallocU`; callers still own it
and free it with `C.free`, including conversion failures. Empty input bypasses
the scratch pool. Native allocation sizes, pointer conversions, function handle
cleanup, connection ownership, public signatures, returned types, SDK error
information and stateful connection semantics are unchanged. This pool is
internal and behaviorally equivalent, so no opt-in API is needed. The ownership
comment at acquisition is retained as required by AGENTS.md despite the general
Go style preference against comments.

`TestInputPoolReuse` exercises concurrent success/error alternation, empty,
Unicode and large inputs, and retained errors. Existing boundary tests cover
embedded NUL, larger strings and malformed UTF-8/UTF-16. The benchmark calls the
actual production converter using the installed native SDK, not an emulation.
Zero steady-state Go allocations does not mean zero C allocations; pool misses
can still allocate Go storage.

## Environment and commands

Apple M3 Pro, darwin/arm64, Go 1.27.1 (module language 1.26), CGO enabled.
Same SDK/environment before and after:

```text
CGO_CFLAGS=-std=gnu17 -I/Users/aden/nwrfcsdk/include
CGO_LDFLAGS=-L/Users/aden/nwrfcsdk/lib -Wl,-rpath,/Users/aden/nwrfcsdk/lib
SHA256 libsapnwrfc.dylib: 052097a0ac6d7b9b629ec79d35bec89f41743909c168a654bf183effac67cbc0
SHA256 libsapucum.dylib: 3ff5380d21776d08c4356f36d73b69439baf157c3c95f9ab181d84e75d48446a
```

Initial baseline and after commands (8 samples each, raw outputs in the locally
ignored `benchmarks/object-pooling/` directory):

```sh
GOMAXPROCS=2 go test ./gorfc -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^$' -bench 'BenchmarkFillString' -benchmem -benchtime 1s -count 8
GOMAXPROCS=4 go test ./gorfc -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^$' -bench '^BenchmarkFillStringParallel$' -benchmem -benchtime 1s -count 8
```

The initial runs showed substantial timing variation and apparent regressions,
so an additional comparison alternated baseline/after binaries, reversing order
on odd iterations, with 8 samples per variant. Both were compiled from the same
benchmark source; the baseline used a Go overlay restoring the original
`gorfc.go` from commit `a8fd9b25f16558a197f7baab470f7a94c5e399f2`.
The overlay maps the absolute repository path of `gorfc/gorfc.go` to
`/private/tmp/gonwrfc-before-pooling.go`, populated with `git show` of that
revision. No benchmark/test jobs from this task ran concurrently.

```sh
go test -c -overlay /private/tmp/gonwrfc-pooling-overlay.json -o /private/tmp/gonwrfc-pooling-before.test ./gorfc
go test -c -o /private/tmp/gonwrfc-pooling-after.test ./gorfc
# Repeat each command eight times, alternating order each iteration:
GOMAXPROCS=2 DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib /private/tmp/gonwrfc-pooling-before.test -test.run='^$' -test.bench=BenchmarkFillString -test.benchmem -test.benchtime=1s -test.count=1
GOMAXPROCS=2 DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib /private/tmp/gonwrfc-pooling-after.test -test.run='^$' -test.bench=BenchmarkFillString -test.benchmem -test.benchtime=1s -test.count=1
```

## Validation

Local tests exclude external SAP integration tests in `gorfc/gorfc_test.go`
except the SDK version test. No staging destination was authorized; neither
integration nor `BenchmarkProduction` load tests were run. Native measurements
do not establish end-to-end SAP latency or throughput improvement.

```sh
local_tests='^(TestNWRFCLibVersion|TestDescriptionStringAllocations|TestStringConversionRoundTrip|TestConversion.*|TestWrapStringGoAllocations|TestNative.*|TestInputPool.*)$'
go test ./... -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run "$local_tests" -count 1
go test -race ./... -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run "$local_tests" -count 1
go test ./... -gcflags=all=-d=checkptr=2 -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run "$local_tests" -count 1
go vet ./...
go test ./... -run '^$' -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib'
go test -asan ./gorfc -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run "$local_tests" -count 1
betteralign ./...
betteralign -apply ./...
betteralign ./...
git diff --check
```

Correctness, race, vet, compile-only and betteralign pass. Betteralign made no
rewrites. Full local checkptr fails the existing `TestDescriptionStringAllocations`
assertions (2 allocations under instrumentation, expected at most 1); no pointer
violation is reported. Go ASan is unsupported on darwin/arm64. No instrumented
SDK or native leak checker was run; these checks do not prove native memory
safety. The linker emits existing SDK search-path/macOS deployment warnings.
The initial sandbox cache-access failure was retried with approved access.

## Initial unbounded-pool experiment (not the final implementation)

All rows have 8 samples per variant. Values are median ns/op [min–max].
The rejected unbounded-pool trial drops from **1800 B/op, 2 allocs/op** to
**0 B/op, 0 allocs/op** in these runs. `benchstat` is unavailable.

### Initial runs

| Benchmark | Before ns/op | After ns/op |
|---|---:|---:|
| BenchmarkFillStringParallel-2 | 423.35 [302–532.2] | 160.35 [132.5–333.4] |
| BenchmarkFillString/ASCII/16-2 | 315.9 [288.4–364.9] | 785.4 [201.7–1082] |
| BenchmarkFillString/ASCII/1024-2 | 1510 [1459–1549] | 2714 [1743–3515] |
| BenchmarkFillString/ASCII/65536-2 | 89096.5 [82785–253090] | 150408 [101962–270769] |
| BenchmarkFillString/Unicode/6144-2 | 7087.5 [6601–16925] | 7384 [6377–27277] |

### Initial parallel run, GOMAXPROCS=4

| Benchmark | Before ns/op | After ns/op |
|---|---:|---:|
| BenchmarkFillStringParallel-4 | 547.65 [259.3–716.4] | 375.25 [225.7–679.7] |

### Alternating verification, GOMAXPROCS=2

| Benchmark | Before ns/op | After ns/op |
|---|---:|---:|
| BenchmarkFillStringParallel-2 | 273.45 [262.5–495.7] | 105.75 [96.69–109.7] |
| BenchmarkFillString/ASCII/16-2 | 254.7 [244.5–376.5] | 149.1 [144.3–178.3] |
| BenchmarkFillString/ASCII/1024-2 | 1420.5 [1301–2361] | 1267.5 [1225–1354] |
| BenchmarkFillString/ASCII/65536-2 | 75343 [74815–115636] | 78448.5 [77060–108790] |
| BenchmarkFillString/Unicode/6144-2 | 6312 [6015–8926] | 6211.5 [6044–6709] |

The large initial sequential timing regressions diminished in the alternating
comparison, but a smaller large-input regression remained. ASCII 16-byte inputs and parallel GOMAXPROCS=2 runs have non-overlapping
sample ranges in that comparison. ASCII 1024-byte, large ASCII and Unicode ranges overlap; no
speed improvement is claimed for them. GOMAXPROCS=4 timing is also noisy and
inconclusive. Allocation reduction is consistent across all samples.

- P2 initial parallel throughput, derived from median ns/op: 2.36 → 6.24 million conversions/s.
- P4 initial parallel throughput, derived from median ns/op: 1.83 → 2.66 million conversions/s.
- P2 alternating parallel throughput, derived from median ns/op: 3.66 → 9.46 million conversions/s.

Large ASCII's alternating median is 4.1% slower. An exact two-sided median
permutation test over all 12,870 allocations of the 16 samples gives p=0.013986.
Overlapping ranges alone were insufficient to reject a regression. The
unbounded implementation was therefore rejected; the final version routes
inputs above 32 KiB through the original converter. Boundary tests cover
32 KiB minus one, exactly 32 KiB and 32 KiB plus one, with valid and malformed
input on each side.

## Final bounded pool

The final implementation retains the unpooled path above 32 KiB.
Each row again contains 8 samples. Commands match the initial runs, with
outputs saved as `final-p2.txt` and `final-p4.txt`. The P2 reference below
is the alternating baseline; P4 uses its initial baseline.

| Benchmark | Baseline median ns/op [range] | Final median ns/op [range] | B/op before → after | allocs/op before → after |
|---|---:|---:|---:|---:|
| BenchmarkFillStringParallel-2 | 273.45 [262.5–495.7] | 96.95 [96.15–98.12] | 1800 → 0 | 2 → 0 |
| BenchmarkFillString/ASCII/16-2 | 254.7 [244.5–376.5] | 149.75 [147.2–156.9] | 1800 → 0 | 2 → 0 |
| BenchmarkFillString/ASCII/1024-2 | 1420.5 [1301–2361] | 1276 [1266–1296] | 1800 → 0 | 2 → 0 |
| BenchmarkFillString/ASCII/65536-2 | 75343 [74815–115636] | 75158 [74611–77868] | 1800 → 1800 | 2 → 2 |
| BenchmarkFillString/Unicode/6144-2 | 6312 [6015–8926] | 6164.5 [6112–6501] | 1800 → 0 | 2 → 0 |
| BenchmarkFillStringParallel-4 | 547.65 [259.3–716.4] | 51.92 [51.21–54.82] | 1800 → 0 | 2 → 0 |

Timing across rounds remains sensitive to machine load; no end-to-end SAP
improvement is claimed. Large-input allocation behavior is preserved. The
small-input allocation reduction is consistent across every measured sample.

Final parallel throughput derived from median timing is 10.31 million
conversions/s at GOMAXPROCS=2 (baseline 3.66 million/s) and 19.26 million/s
at GOMAXPROCS=4 (initial noisy baseline 1.83 million/s). These are local native
conversion measurements, and the noisy P4 baseline prevents attributing the
whole timing difference to pooling.

The focused checkptr command excludes only the pre-existing allocation-count
test; all local conversion, metadata and new pool tests remain selected:

```sh
go test ./... -gcflags=all=-d=checkptr=2 -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^(TestNWRFCLibVersion|TestStringConversionRoundTrip|TestConversion.*|TestWrapStringGoAllocations|TestNative.*|TestInputPool.*)$' -count 1
```

Final validation: local correctness, race, focused checkptr, vet, compile-only,
betteralign and diff whitespace checks pass. Full local checkptr still fails
only the five existing allocation-count assertions. Final logs are retained as
`final-test.txt`, `final-race.txt`, `final-checkptr.txt`,
`final-checkptr-conversion.txt`, `final-vet.txt`, `final-compile.txt` and
`betteralign-final*.txt` in `benchmarks/object-pooling/`. ASan remains unsupported;
SAP integration/load tests remain unrun without an authorized destination.
