package gorfc

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestInputPoolBoundary(t *testing.T) {
	for _, size := range []int{(32 << 10) - 1, 32 << 10, (32 << 10) + 1} {
		for _, suffix := range []string{"x", "\xff"} {
			input := strings.Repeat("a", size-1) + suffix
			converted, length, err := fillStringWithLength(input)
			if suffix == "\xff" {
				freeSAPUC(converted)
				var sdkErr *RfcError
				if !errors.As(err, &sdkErr) {
					t.Fatalf("size %d: expected SDK error, got %v", size, err)
				}

				continue
			}

			if err != nil {
				freeSAPUC(converted)
				t.Fatal(err)
			}

			got, err := nWrapString(converted, length, false)
			freeSAPUC(converted)
			if err != nil || got != input {
				t.Fatalf("size %d: round trip mismatch, error %v", size, err)
			}
		}
	}
}

func TestInputPoolReuse(t *testing.T) {
	for worker := range 8 {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()

			var previous *RfcError
			var previousText string
			for i := range 32 {
				for _, input := range []string{"\xff", "", fmt.Sprintf("%d:%d:é😀", worker, i), "\xc0\xaf", strings.Repeat("a😀é", 32768)} {
					converted, length, err := fillStringWithLength(input)
					if input == "\xff" || input == "\xc0\xaf" {
						freeSAPUC(converted)
						var sdkErr *RfcError
						if !errors.As(err, &sdkErr) || sdkErr.ErrorInfo.Code == "" {
							t.Fatalf("expected SDK error, got %v", err)
						}

						if previous != nil && previous.Error() != previousText {
							t.Fatal("pool reuse changed a returned error")
						}

						previous, previousText = sdkErr, sdkErr.Error()
						continue
					}

					if err != nil {
						freeSAPUC(converted)
						t.Fatal(err)
					}

					got, err := nWrapString(converted, length, false)
					freeSAPUC(converted)
					if err != nil || got != input {
						t.Fatalf("round trip mismatch: error %v", err)
					}
				}
			}
		})
	}
}
