package gorfc

import (
	"errors"
	"strings"
	"testing"
	"unsafe"
)

func TestWrapStringGoAllocations(t *testing.T) {
	for _, input := range []string{"", "STFC_CONNECTION", "é😀", strings.Repeat("x", 65536)} {
		converted, length, err := fillStringWithLength(input)
		if err != nil {
			freeSAPUC(converted)
			t.Fatal(err)
		}
		t.Cleanup(func() { freeSAPUC(converted) })
		want := float64(1)
		if input == "" {
			want = 0
		}
		allocs := testing.AllocsPerRun(100, func() {
			var err error
			stringResult, err = nWrapString(converted, length, false)
			if err != nil {
				t.Fatal(err)
			}
		})
		if allocs != want {
			t.Fatalf("input of %d bytes: %.0f Go allocations, want %.0f", len(input), allocs, want)
		}
	}
}

func TestConversionBoundaries(t *testing.T) {
	for _, input := range []string{"", "a\x00b", strings.Repeat("a😀é", 1<<18)} {
		converted, length, err := fillStringWithLength(input)
		if err != nil {
			freeSAPUC(converted)
			t.Fatal(err)
		}
		got, err := nWrapString(converted, length, false)
		freeSAPUC(converted)
		if err != nil || got != input {
			t.Fatalf("round trip of %d bytes: error %v, equal %t", len(input), err, got == input)
		}
	}

	for _, input := range []string{"\xff", "\xc0\xaf", "\xf0\x9f\x98"} {
		converted, _, err := fillStringWithLength(input)

		freeSAPUC(converted)
		var sdkErr *RfcError
		if !errors.As(err, &sdkErr) || sdkErr.ErrorInfo.Code == "" {
			t.Fatalf("invalid UTF-8 %x: expected SDK error, got %v", input, err)
		}
	}

	converted, length, err := fillStringWithLength("x")
	if err != nil {
		freeSAPUC(converted)
		t.Fatal(err)
	}
	defer freeSAPUC(converted)

	*(*uint16)(unsafe.Pointer(converted)) = 0xd800
	if _, err := nWrapString(converted, length, false); err == nil {
		t.Fatal("expected SDK conversion failure for an unpaired surrogate")
	}
	if got, err := nWrapString(nil, 0, true); err != nil || got != "" {
		t.Fatalf("empty input: %q, %v", got, err)
	}
}

func TestConversionStrip(t *testing.T) {
	for _, input := range []string{"", "  ", "value \x00", "é😀  ", "a\x00b "} {
		converted, length, err := fillStringWithLength(input)
		if err != nil {
			freeSAPUC(converted)
			t.Fatal(err)
		}
		got, err := nWrapString(converted, length, true)
		freeSAPUC(converted)
		if want := strings.TrimRight(input, "\x00 "); err != nil || got != want {
			t.Fatalf("strip %q: got %q, want %q, error %v", input, got, want, err)
		}
	}
}
