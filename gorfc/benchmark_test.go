package gorfc

import (
	"strconv"
	"strings"
	"testing"
)

var benchmarkString string

func BenchmarkFillString(b *testing.B) {
	for _, size := range []int{16, 1024, 65536} {
		value := strings.Repeat("a", size)
		b.Run("ASCII/"+strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(value)))
			for b.Loop() {
				converted, _, err := fillStringWithLength(value)
				if err != nil {
					b.Fatal(err)
				}
				freeSAPUC(converted)
			}
		})
	}

	value := strings.Repeat("😀é", 1024)
	b.Run("Unicode/"+strconv.Itoa(len(value)), func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(value)))
		for b.Loop() {
			converted, _, err := fillStringWithLength(value)
			if err != nil {
				b.Fatal(err)
			}
			freeSAPUC(converted)
		}
	})
}

func BenchmarkWrapString(b *testing.B) {
	for _, size := range []int{16, 1024, 65536} {
		value := strings.Repeat("a", size)
		converted, length, err := fillStringWithLength(value)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { freeSAPUC(converted) })

		b.Run("ASCII/"+strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(value)))
			for b.Loop() {
				benchmarkString, err = nWrapString(converted, length, false)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
