package paac

import (
	"fmt"
	"testing"
)

var result_test_struct_30 TestStruct30

func benchmarkMap(count int, b *testing.B) {
	m := make(map[string]TestStruct30, count)
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		key := "Key" + fmt.Sprint(i)
		keys[i] = key
		m[key] = NewTestStruct30(i, b.N)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result_test_struct_30 = m["Key"+fmt.Sprint(i%count)]
	}
}

func BenchmarkMap100(b *testing.B) {
	benchmarkMap(100, b)
}

func BenchmarkMap1000(b *testing.B) {
	benchmarkMap(1000, b)
}

func BenchmarkMap10000(b *testing.B) {
	benchmarkMap(10000, b)
}

func BenchmarkMap100000(b *testing.B) {
	benchmarkMap(100000, b)
}

func BenchmarkMap1000000(b *testing.B) {
	benchmarkMap(1000000, b)
}

// func BenchmarkMap10000000(b *testing.B) {
// 	benchmarkMap(10000000, b)
// }
