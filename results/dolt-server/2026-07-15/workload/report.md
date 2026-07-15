# Storage spike run: dolt-server

- Dataset SHA-256: `89e2e35498bfa091feab5004e6bf4a5fb0b984b6f241260836cf26498f4eaa04`
- Backend version: `dolt version 2.2.0`
- Driver version: `github.com/go-sql-driver/mysql v1.9.3`
- Go/platform: `go1.26.5` on `linux/amd64`
- Batch size: 100
- Started: 2026-07-15T22:23:56.568832801Z
- Finished: 2026-07-15T22:24:52.54020749Z
- Disk footprint: 9850 → 170181869 bytes (delta +170172019)

## Phases

- `load_sources`: 1000 operations in 10 batches; p50=186.352121ms, p95=459.930339ms, p99=459.930339ms, throughput=361.71 ops/s
- `load_claims`: 10000 operations in 100 batches; p50=442.068799ms, p95=537.257408ms, p99=562.243927ms, throughput=229.31 ops/s
- `query_claims`: 10000 operations in 10000 batches; p50=952.957µs, p95=1.022767ms, p99=1.0615ms, throughput=1042.40 ops/s
