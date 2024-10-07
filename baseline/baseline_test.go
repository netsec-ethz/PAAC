package paac

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"testing"
)

var result_bool bool

func benchmarkBaselineMapParallel(policy_i, num_rules, eval_size, num_files int, b *testing.B) {
	fname := fmt.Sprintf("bench_files/policies/tmp/%s_%d_%d_%d_%d.csv", "benchmark_policy", num_rules, eval_size, num_files, policy_i)
	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		BuildPolicyCsv(num_rules, eval_size, num_files, fname)
	}

	e, err := NewBaselineEnforcer("bench_files/abac_model.conf", fname)
	if err != nil {
		log.Fatalln("ERROR: Enforcer creation returned error: ", err)
	}
	if fmt.Sprintf("%T", e.Enforcer) != "*casbin.SyncedEnforcer" {
		log.Fatalln("Parallel benchmark called on non-synced Enforcer")
	}

	data := make([]map[string]any, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = NewTestMap(10, i, b.N)
	}

	var bi atomic.Uint64

	b.SetParallelism(2)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(bi.Add(1)) - 1
			result_bool, err = e.Enforce(data[i], fmt.Sprintf("data%v", i%num_files), "read")
			if err != nil {
				fmt.Printf("ERROR: Got enforcement error: %s\n For request %s", err, data[i])
			}
		}
	})
	os.Remove(fname)
}

func BenchmarkBaselineMapParallelRunner(b *testing.B) {
	rules := []int{1, 10, 100, 1000, 10000, 100000}
	evals := []int{1, 3, 5, 10}
	files := []int{1, 10, 100, 1000}
	for i := 0; i < 200; i++ {
		for _, r := range rules {
			for _, e := range evals {
				for _, f := range files {
					if f <= r {
						name := fmt.Sprintf("BenchmarkBaselineMapParallel30Attrs%dRules%dEvals%dFiles", r, e, f)
						benchFunc := func(barg *testing.B) {
							benchmarkBaselineMapParallel(i, r, e, f, barg)
						}
						b.Run(name, benchFunc)
					}
				}
			}
		}
	}
}

// go test baseline_utils.go baseline.go baseline_test.go -timeout 20m -bench BenchmarkBaselineJsonRunner -count 6 | tee bench_json_count_6.txt
func benchmarkBaselineJson(policy_i, num_rules, eval_size, num_files int, b *testing.B) {
	fname := fmt.Sprintf("bench_files/policies/set%d/%s_%d_%d_%d.csv", policy_i, "benchmark_policy", num_rules, eval_size, num_files)
	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		BuildPolicyCsv(num_rules, eval_size, num_files, fname)
	}

	e, _ := NewBaselineEnforcer("bench_files/abac_model.conf", fname)
	data := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		js, _ := json.Marshal(NewTestStruct30(i, b.N))
		data[i] = string(js)
	}

	e.EnableAcceptJsonRequest(true)
	b.ResetTimer()

	var err error
	for i := 0; i < b.N; i++ {
		result_bool, err = e.Enforce(data[i], fmt.Sprintf("data%v", i%num_files), "read")
		if err != nil {
			log.Fatalf("Got enforcement error: %s\n", err)
		}
	}
}

func BenchmarkBaselineJsonRunner(b *testing.B) {
	rules := []int{1, 10, 100, 1000, 10000, 100000}
	evals := []int{1, 3, 5, 10}
	files := []int{1, 10, 100, 1000}
	for _, r := range rules {
		for _, e := range evals {
			for _, f := range files {
				if f <= r {
					for i := 0; i < 6; i++ {
						name := fmt.Sprintf("BenchmarkBaselineJson30Attrs%dRules%dEvals%dFiles", r, e, f)
						benchFunc := func(barg *testing.B) {
							benchmarkBaselineJson(i, r, e, f, barg)
						}
						b.Run(name, benchFunc)

					}
				}
			}
		}
	}
}

// go test baseline_utils.go baseline.go baseline_test.go -timeout 20m -bench BenchmarkBaselineStructRunner -count 6 | tee bench_struct_count_6.txt
func benchmarkBaselineStruct(policy_i, num_rules, eval_size, num_files int, b *testing.B) {
	fname := fmt.Sprintf("bench_files/policies/set%d/%s_%d_%d_%d.csv", policy_i, "benchmark_policy", num_rules, eval_size, num_files)
	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		BuildPolicyCsv(num_rules, eval_size, num_files, fname)
	}

	e, _ := NewBaselineEnforcer("bench_files/abac_model.conf", fname)
	data := make([]TestStruct30, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = NewTestStruct30(i, b.N)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result_bool, _ = e.Enforce(data[i], fmt.Sprintf("data%v", i%num_files), "read")
	}
}

func BenchmarkBaselineStructRunner(b *testing.B) {
	rules := []int{1, 10, 100, 1000, 10000, 100000}
	evals := []int{1, 3, 5, 10}
	files := []int{1, 10, 100, 1000}
	for _, r := range rules {
		for _, e := range evals {
			for _, f := range files {
				if f <= r {
					for i := 0; i < 6; i++ {
						name := fmt.Sprintf("BenchmarkBaselineStruct30Attrs%dRules%dEvals%dFiles", r, e, f)
						benchFunc := func(barg *testing.B) {
							benchmarkBaselineStruct(i, r, e, f, barg)
						}
						b.Run(name, benchFunc)

					}
				}
			}
		}
	}
}

func benchmarkBaselineMap(policy_i, num_rules, eval_size, num_files int, b *testing.B) {
	fname := fmt.Sprintf("bench_files/policies/set%d/%s_%d_%d_%d.csv", policy_i, "benchmark_policy", num_rules, eval_size, num_files)
	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		BuildPolicyCsv(num_rules, eval_size, num_files, fname)
	}

	e, _ := NewBaselineEnforcer("bench_files/abac_model.conf", fname)
	data := make([]map[string]any, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = NewTestMap(10, i, b.N)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result_bool, _ = e.Enforce(data[i], fmt.Sprintf("data%v", i%num_files), "read")
	}
}

func BenchmarkBaselineMapRunner(b *testing.B) {
	rules := []int{1, 10, 100, 1000, 10000, 100000}
	evals := []int{1, 3, 5, 10}
	files := []int{1, 10, 100, 1000}
	for _, r := range rules {
		for _, e := range evals {
			for _, f := range files {
				if f <= r {
					for i := 0; i < 10; i++ {
						name := fmt.Sprintf("BenchmarkBaselineStruct30Attrs%dRules%dEvals%dFiles", r, e, f)
						benchFunc := func(barg *testing.B) {
							benchmarkBaselineMap(i, r, e, f, barg)
						}
						b.Run(name, benchFunc)

					}
				}
			}
		}
	}
}

func benchmarkBaselineStructParallel(policy_i, num_rules, eval_size, num_files int, b *testing.B) {
	fname := fmt.Sprintf("bench_files/policies/set%d/%s_%d_%d_%d.csv", policy_i, "benchmark_policy", num_rules, eval_size, num_files)
	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		BuildPolicyCsv(num_rules, eval_size, num_files, fname)
	}

	e, _ := NewBaselineEnforcer("bench_files/abac_model.conf", fname)
	if fmt.Sprintf("%T", e.Enforcer) != "*casbin.SyncedEnforcer" {
		log.Fatalln("Parallel benchmark called on non-synced Enforcer")
	}
	data := make([]TestStruct30, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = NewTestStruct30(i, b.N)
	}

	var bi atomic.Uint64

	b.SetParallelism(2)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(bi.Add(1)) - 1
			result_bool, _ = e.Enforce(data[i], fmt.Sprintf("data%v", i%num_files), "read")
		}
	})
}

func BenchmarkBaselineStructParallelRunner(b *testing.B) {
	rules := []int{1, 10, 100, 1000, 10000, 100000}
	evals := []int{1, 3, 5, 10}
	files := []int{1, 10, 100, 1000}
	for _, r := range rules {
		for _, e := range evals {
			for _, f := range files {
				if f <= r {
					for i := 0; i < 6; i++ {
						name := fmt.Sprintf("BenchmarkBaselineStructParallel30Attrs%dRules%dEvals%dFiles", r, e, f)
						benchFunc := func(barg *testing.B) {
							benchmarkBaselineStructParallel(i, r, e, f, barg)
						}
						b.Run(name, benchFunc)
					}
				}
			}
		}
	}
}
