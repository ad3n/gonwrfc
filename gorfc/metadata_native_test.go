//go:build gorfc_native_test && cgo

package gorfc

import (
	"errors"
	"strings"
	"testing"
)

func TestNativeTypeDescriptionName(t *testing.T) {
	for _, name := range []string{"", "Z_SHORT", strings.Repeat("A", 20), strings.Repeat("B", 21), strings.Repeat("C", 30), strings.Repeat("界", 30)} {
		t.Run(name, func(t *testing.T) {
			wrap, destroy, err := nativeTypeDescriptionFixture(name)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := destroy(); err != nil {
					t.Error(err)
				}
			})
			got, err := wrap()
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != name || got.NucLength != 0 || got.UcLength != 0 || len(got.Fields) != 0 {
				t.Fatalf("unexpected description: %#v; want name %q and empty layout", got, name)
			}
		})
	}
}

func TestNativeTypeDescriptionInvalidHandle(t *testing.T) {
	_, err := wrapTypeDescription(nil)
	var sdkErr *RfcError
	if !errors.As(err, &sdkErr) || sdkErr.ErrorInfo.Code == "" {
		t.Fatalf("expected SDK error for nil handle, got %v", err)
	}
}

func TestNativeTypeDescriptionMalformedName(t *testing.T) {
	_, destroy, err := nativeTypeDescriptionFixture("\xff")
	if destroy != nil {
		t.Cleanup(func() { _ = destroy() })
	}
	var sdkErr *RfcError
	if !errors.As(err, &sdkErr) {
		t.Fatalf("expected SDK conversion error, got %v", err)
	}
}

var benchmarkTypeDescription TypeDescription

func BenchmarkNativeWrapTypeDescription(b *testing.B) {
	wrap, destroy, err := nativeTypeDescriptionFixture("Z_SHORT")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := destroy(); err != nil {
			b.Error(err)
		}
	})
	b.ReportAllocs()
	for b.Loop() {
		benchmarkTypeDescription, err = wrap()
		if err != nil {
			b.Fatal(err)
		}
	}
}
