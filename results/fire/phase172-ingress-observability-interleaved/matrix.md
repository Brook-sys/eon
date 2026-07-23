# Interleaved sustained-ingress observability smoke matrix

- Schema: `motor-autonomo.sustained-ingress-observability-matrix.v1`
- Schedule: `50,100,200,100,200,50` (`rotating_latin_v1`, two blocks).
- Production delay: unchanged at 100 ms.

| Delay | runs | convergence ms | exhaustion rate | attempts | outcomes |
|---:|---:|---:|---:|---:|---|
| 50 | 2 | 4224/3911 | 0.083/0.125 | 48/52 | {'cas_conflict': 57, 'success': 43} |
| 100 | 2 | 4152/3972 | 0.167/0.083 | 53/51 | {'cas_conflict': 62, 'success': 42} |
| 200 | 2 | 4435/4747 | 0.125/0.208 | 51/52 | {'cas_conflict': 63, 'success': 40} |

## Interpretation

The deterministic interleaving and per-attempt trace are operational. This bounded smoke matrix is not a tuning result: two runs per cell cannot separate delay effects from host scheduling. No production setting changed.

## Next experiment

Run the same rotating schedule for at least 20 blocks, preserving load snapshots and attempt traces; compare normalized host load and CAS/SQLite timing distributions before considering pacing changes.
