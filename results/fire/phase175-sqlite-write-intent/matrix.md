# Phase 175 — SQLite write-intent contention comparison

Isolated experiment comparing the current deferred `BeginTx` path with a `BEGIN IMMEDIATE` (_txlock=immediate) path that acquires SQLite write intent before the callback/checkpoint work. Production `Store.Update` is unchanged; the immediate path is test-only.

| mode | outcome | n | begin p50/p95 us | write+CAS p50/p95 us | transaction-open p50/p95 us | reload p50/p95 us |
|---|---|---:|---:|---:|---:|---:|
| deferred | cas_conflict | 18 | 36/43 | 329811/329985 | 329779/329890 | 1460/2091 |
| immediate_before_callback | cas_conflict | 18 | 379858/529967 | 163/213 | 3871/313567 | 1240/1743 |

Both modes ran 4 workers × 6 cycles with one transaction attempt per worker-cycle, a 300 ms leader hold, rotating leadership, and the same SQLite `busy_timeout` default.

Interpretation: `BEGIN IMMEDIATE` shifts the contention from `write+CAS` (330 ms p50) into `begin` (380 ms p50) — the total transaction-open time is essentially unchanged (~330 ms for deferred, ~313 ms p95 for immediate). The write phase after acquiring the immediate lock is very fast (163 µs p50) because the lock is already held, but followers still wait the same total time for the leader to commit. Evicted contention is not reduced; it is merely relocated.

Decision: do not adopt `BEGIN IMMEDIATE` in production. The current deferred path is simpler, separates callback work from lock acquisition, and produces the same total contention profile. The next experiment should focus on reducing the hold time itself (the 300 ms failpoint delay simulates a slow callback or large payload serialisation), not on where the wait appears in the phase decomposition.
