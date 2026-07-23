# Repeated sustained-ingress recovery-delay matrix

- Schema: `motor-autonomo.sustained-ingress-delay-matrix.v2`
- Bounds: 50/100/200 ms, 5 isolated runs each, 15 campaigns total.
- Production policy: unchanged at 100 ms.

| Delay | convergence ms min/p50/p95/max | exhaustion rate min/p50/p95/max | attempts min/p50/p95/max | fair+converged |
|---:|---:|---:|---:|---:|
| 50 | 3922/3984/4221/4221 | 0.125/0.125/0.167/0.167 | 51/51/52/52 | 5/5 |
| 100 | 4036/4225/4533/4533 | 0.125/0.167/0.208/0.208 | 52/53/53/53 | 5/5 |
| 200 | 4400/4508/10493/10493 | 0.042/0.167/0.208/0.208 | 49/52/53/53 | 5/5 |

## Interpretation

Keep the production delay at 100 ms. All 15 runs converged fairly. The 50 ms cell had the lowest median convergence, but its exhaustion range overlaps 100 ms and one 200 ms scheduler outlier dominated p95; the evidence is not strong enough to tune production.

## Next experiment

Randomize/interleave delay order and increase to at least 20 runs per cell, recording host load and per-attempt BUSY/CAS timing before considering a production change.
