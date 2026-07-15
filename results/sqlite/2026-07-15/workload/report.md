# Storage spike run: sqlite

- Dataset SHA-256: `89e2e35498bfa091feab5004e6bf4a5fb0b984b6f241260836cf26498f4eaa04`
- Backend version: `SQLite 3.50.4`
- Driver version: `modernc.org/sqlite v1.39.1`
- Go/platform: `go1.26.5` on `linux/amd64`
- Batch size: 100
- Started: 2026-07-15T22:26:15.604685767Z
- Finished: 2026-07-15T22:27:31.659607553Z
- Disk footprint: 45136 → 43666696 bytes (delta +43621560)

## Phases

- `load_sources`: 1000 operations in 10 batches; p50=162.56241ms, p95=288.084075ms, p99=288.084075ms, throughput=659.13 ops/s
- `load_claims`: 10000 operations in 100 batches; p50=775.689285ms, p95=939.66713ms, p99=1.008683521s, throughput=154.42 ops/s
- `query_claims`: 10000 operations in 10000 batches; p50=965.126µs, p95=1.068461ms, p99=1.272341ms, throughput=1022.85 ops/s
