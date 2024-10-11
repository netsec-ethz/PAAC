go test -bench="^BenchmarkBaseline$" -timeout=36000s -args -scionDir="/home/your_scion_path_here"| tee bench_files/results/BenchmarkBaseline.txt 
go test -bench="^BenchmarkBaselineParallel$" -timeout=36000s -args -scionDir="/home/your_scion_path_here"| tee bench_files/results/BenchmarkBaselineParallel.txt 
go test -bench="^BenchmarkACS$" -timeout=36000s -args -scionDir="/home/your_scion_path_here" | tee bench_files/results/BenchmarkACS.txt 
go test -bench="^BenchmarkPAACNoNet$" -timeout=36000s -args -scionDir="/home/your_scion_path_here" | tee bench_files/results/BenchmarkPAACNoNet.txt 
go test -bench="^BenchmarkPAACNet$" -timeout=36000s -args -scionDir="/home/your_scion_path_here"| tee bench_files/results/BenchmarkPAACNet.txt 
go test -bench="^BenchmarkPAACNoNetNoCache$" -timeout=36000s -args -scionDir="/home/your_scion_path_here"| tee bench_files/results/BenchmarkPAACNoNetNoCache.txt 
go test -bench="^BenchmarkPAACSinglePath$" -timeout=36000s -args -scionDir="/home/your_scion_path_here" | tee bench_files/results/BenchmarkPAACSinglePath.txt 
