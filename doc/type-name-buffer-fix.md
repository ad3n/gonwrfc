# Type-name buffer fix

The affected production path is Connection.GetFunctionDescription →
wrapFunctionDescription → wrapTypeDescription (including nested types).
RfcGetTypeName requires RFC_ABAP_NAME, a 31-element RFC_CHAR array. The old
malloc(41) allocated bytes, not Unicode characters. The new allocation uses
C.sizeof_RFC_ABAP_NAME (62 bytes on the installed SDK), including the terminator.
No public signature, returned type, connection behavior, or locking changes.

The C buffer is owned by wrapTypeDescription. Its existing deferred C.free runs
on success and all returned errors; wrapString copies into Go-owned storage
before that free. Descriptor ownership is unchanged. The test fixture creates
an uncached native descriptor, registers deterministic destruction, and sets its
handle to nil after successful destruction. The input-name conversion allocation
is freed even on error. No Go pointer is retained by C.

## Regression coverage

gorfc/metadata_native_test.go runs against the real local SAP SDK with the
gorfc_native_test build tag. Its small CGO fixture bridge is excluded from
normal builds because Go does not support import C in _test.go files.
Coverage: empty/short names, 20 and 21 characters around the old capacity,
maximum 30-character ASCII and Unicode names, nil-handle SDK error, and malformed
UTF-8 rejected during fixture setup. The benchmark calls the actual production
wrapper, not a rewritten implementation. It uses a short name so the unfixed
baseline does not intentionally overrun memory.

Later metadata enumeration errors and allocation exhaustion were not
fault-injected. Their existing shared defer was reviewed and remains unchanged.
The malformed UTF-8 test covers fixture input conversion, not an SDK-generated
malformed type name. No claim of exhaustive SDK error simulation is made.

## Measurement

Apple M3 Pro, darwin/arm64, Go 1.27.1, CGO enabled, GOMAXPROCS=2.
CGO_CFLAGS=-std=gnu17 -I/Users/aden/nwrfcsdk/include
CGO_LDFLAGS=-L/Users/aden/nwrfcsdk/lib -Wl,-rpath,/Users/aden/nwrfcsdk/lib

SDK SHA256:
- libsapnwrfc.dylib: 052097a0ac6d7b9b629ec79d35bec89f41743909c168a654bf183effac67cbc0
- libsapucum.dylib: 3ff5380d21776d08c4356f36d73b69439baf157c3c95f9ab181d84e75d48446a

Identical command before/after, sequential runs without concurrent test jobs:

```sh
GOMAXPROCS=2 go test ./gorfc -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^$' -bench '^BenchmarkNativeWrapTypeDescription$' -benchmem -benchtime 1s -count 8
```

| Metric | Before | After |
|---|---:|---:|
| Samples | 8 | 8 |
| Median ns/op | 443.2 | 457.8 |
| Min–max ns/op | 408.2–510.3 | 400.1–866.6 |
| B/op (all samples) | 1928 | 1928 |
| allocs/op (all samples) | 4 | 4 |

Timing is noisy and ranges overlap; no performance improvement or statistically
established regression is claimed. This is a memory-safety correction, not a
zero-allocation optimization. The intentional native allocation increases from
41 bytes to sizeof(RFC_ABAP_NAME); Go benchmem does not measure that allocation.
Raw data is retained locally under benchmarks/type-name-fix/ (gitignored).

## Checks and limitations

Local correctness (including new native tests), race, tagged/default compilation,
and tagged vet pass. Full local checkptr fails the pre-existing
TestDescriptionStringAllocations assertions (2 allocations versus limit 1);
no pointer violation was reported. The focused checkptr run, excluding only
that formatter allocation test, passes. No assertion was weakened.
Go ASan rejects darwin/arm64. The SDK itself is not sanitizer-instrumented.

Native leak checking is not a clean pass. Initial launcher attempts either
could not attach to env or lost DYLD_LIBRARY_PATH; a zero wrapper exit status
in leaks-direct/leaks-verbose was not counted as validation. The final run below
actually executes and passes every TestNative case, but leaks exits 1 and reports
one 48-byte xpc_date_t root leak, with a restricted-process inspection warning.
That result does not prove the changed allocation leaked or establish native
leak freedom. See benchmarks/type-name-fix/leaks-final.txt for the full stack.

```sh
go test -c ./gorfc -tags gorfc_native_test -o benchmarks/type-name-fix/gorfc.test
ln -s /Users/aden/nwrfcsdk/lib/libsapnwrfc.dylib benchmarks/type-name-fix/libsapnwrfc.dylib
ln -s /Users/aden/nwrfcsdk/lib/libsapucum.dylib benchmarks/type-name-fix/libsapucum.dylib
leaks --atExit -- benchmarks/type-name-fix/gorfc.test -test.run '^TestNative' -test.v -test.count 1
```

SAP integration tests in gorfc/gorfc_test.go except TestNWRFCLibVersion are
excluded: no external destination was supplied/authorized. No SAP load benchmark
was run; this change does not alter Call/network/pool paths. Local SDK validation
is not end-to-end SAP validation. Existing linker warnings mention missing default
SDK paths and differing macOS deployment targets.

Exact check commands and matching local output filenames:

### test.txt

```sh
go test ./... -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^(TestNWRFCLibVersion|TestDescriptionStringAllocations|TestStringConversionRoundTrip|TestConversion.*|TestWrapStringGoAllocations|TestNative.*)$' -count 1
```

### race.txt

```sh
go test -race ./... -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^(TestNWRFCLibVersion|TestDescriptionStringAllocations|TestStringConversionRoundTrip|TestConversion.*|TestWrapStringGoAllocations|TestNative.*)$' -count 1
```

### checkptr.txt

```sh
go test ./... -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^(TestNWRFCLibVersion|TestDescriptionStringAllocations|TestStringConversionRoundTrip|TestConversion.*|TestWrapStringGoAllocations|TestNative.*)$' -gcflags=all=-d=checkptr=2 -count 1
```

### compile.txt

```sh
go test ./... -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^$'
```

### vet.txt

```sh
go vet -tags gorfc_native_test ./...
```

### asan.txt

```sh
go test -asan ./gorfc -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^TestNative' -count 1
```

### checkptr-focused.txt

```sh
go test ./... -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^(TestNWRFCLibVersion|TestStringConversionRoundTrip|TestConversion.*|TestWrapStringGoAllocations|TestNative.*)$' -gcflags=all=-d=checkptr=2 -count 1
```

### leaks.txt

```sh
go test ./gorfc -tags gorfc_native_test -exec 'leaks --atExit -- env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^TestNative' -count 1
```

### compile-default.txt

```sh
go test ./... -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib' -run '^$'
```

### leaks-direct.txt

```sh
go test ./gorfc -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib leaks --atExit --' -run '^TestNative' -count 1
```

### leaks-verbose.txt

```sh
go test -v ./gorfc -tags gorfc_native_test -exec 'env DYLD_LIBRARY_PATH=/Users/aden/nwrfcsdk/lib leaks --atExit --' -run '^TestNative' -count 1
```
