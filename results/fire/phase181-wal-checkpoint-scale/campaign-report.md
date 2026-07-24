# Phase 181 — WAL checkpoint scale

Hypothesis: crossing the 1000-page automatic-checkpoint threshold bounds WAL growth and reopen latency; disabling it makes explicit TRUNCATE valuable.

Scenario: 16 isolated cells, FULL/NORMAL × auto-checkpoint 1000/disabled × 500/2000 sequential commits × passive close/explicit TRUNCATE. One run per cell; bounded at 600 s.

## Observations

- Default checkpointing bounded pre-close WAL to 4.18–4.35 MB; disabled checkpointing grew it to 27.4 MB at 500 commits and 285.5 MB at 2000.
- Worst passive reopen was 1,376 ms (NORMAL, disabled, 2000). The paired explicit-TRUNCATE cell reopened in 58 ms, a 23.7× reduction; checkpoint cost is included separately by the harness but this initial console-only run did not preserve that field.
- At the default threshold, passive reopen ranged 41–431 ms; paired TRUNCATE reopen ranged 18–127 ms.
- All 16 cells reopened and exposed every idempotency record.
- Commit duration dominated total cost and had high between-cell variance; a single ordered run cannot authorize tuning.

## Decision

Keep production at synchronous=FULL and wal_autocheckpoint=1000. Explicit TRUNCATE is useful as an operational experiment, not yet an automatic runtime policy. The next rerun must persist checkpoint duration/result directly and randomize paired cell order before comparing total close+checkpoint+reopen cost.
