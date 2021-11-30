go test -bench=BenchmarkFederationBatchingMaster -benchtime=30s -count 1 -benchmem -cpuprofile=cpu_master.out -memprofile=mem_master.out;
go test -bench=BenchmarkFederationBatchingFastJson -benchtime=30s -count 1 -benchmem -cpuprofile=cpu_fast.out -memprofile=mem_fast.out;
