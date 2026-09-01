package gorfc

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const benchmarkDestinationEnv = "GORFC_BENCH_DEST"

func benchmarkDestination(b *testing.B) string {
	b.Helper()
	destination := os.Getenv(benchmarkDestinationEnv)
	if destination == "" {
		b.Skipf("set %s to run benchmarks against an explicitly selected SAP destination", benchmarkDestinationEnv)
	}
	return destination
}

func verifyBenchmarkConnection(b *testing.B, conn *Connection, payload string) {
	b.Helper()
	result, err := conn.Call("STFC_CONNECTION", map[string]any{"REQUTEXT": payload})
	if err != nil {
		b.Fatalf("STFC_CONNECTION smoke call failed: %v", err)
	}
	if got, ok := result["ECHOTEXT"].(string); !ok || got != payload {
		b.Fatalf("unexpected ECHOTEXT: got %#v, want %q", result["ECHOTEXT"], payload)
	}
}

// BenchmarkProductionCall measures the complete production Call path: SDK
// lookup, input conversion, network invocation, and output conversion. It is
// opt-in because every iteration invokes the configured SAP system.
func BenchmarkProductionCall(b *testing.B) {
	destination := benchmarkDestination(b)
	conn, err := ConnectionFromDest(destination)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = conn.Close() })

	// STFC_CONNECTION uses a fixed-width field, so keep payloads within the
	// standard function's portable limit. Large STRING conversion is covered
	// separately by BenchmarkFillString and BenchmarkWrapString.
	for _, size := range []int{32, 255} {
		payload := strings.Repeat("a", size)
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			verifyBenchmarkConnection(b, conn, payload)
			params := map[string]any{"REQUTEXT": payload}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for b.Loop() {
				if _, err := conn.Call("STFC_CONNECTION", params); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkProductionPool measures concurrent steady-state throughput. Pool
// creation and connection establishment are deliberately outside the timer.
func BenchmarkProductionPool(b *testing.B) {
	destination := benchmarkDestination(b)
	maxOpen := runtime.GOMAXPROCS(0)
	pool, err := NewConnectionPool(
		ConnectionParameters{"dest": destination},
		PoolConfig{MaxOpen: maxOpen, MaxIdle: maxOpen},
	)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = pool.Close() })

	// Establish all sessions before timing so this measures pool reuse and RFC
	// throughput rather than one-time logon latency.
	warmed := make([]*Connection, 0, maxOpen)
	for range maxOpen {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		warmed = append(warmed, conn)
	}
	for _, conn := range warmed {
		if err := pool.Release(conn); err != nil {
			b.Fatal(err)
		}
	}

	payload := strings.Repeat("a", 255)
	params := map[string]any{"REQUTEXT": payload}
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	verifyBenchmarkConnection(b, conn, payload)
	if err := pool.Release(conn); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Acquire(context.Background())
			if err != nil {
				b.Error(err)
				return
			}
			_, callErr := conn.Call("STFC_CONNECTION", params)
			releaseErr := pool.Release(conn)
			if callErr != nil || releaseErr != nil {
				b.Errorf("call error: %v; release error: %v", callErr, releaseErr)
				return
			}
		}
	})
}
