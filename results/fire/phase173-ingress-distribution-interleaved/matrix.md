# Phase 173 — sustained ingress distribution matrix

- Schema: `motor-autonomo.sustained-ingress-observability-matrix.v2`.
- Schedule: 20 balanced rotating blocks, 60 isolated campaigns.
- Production recovery delay remained fixed at 100 ms; all delay overrides were test-only.

| delay | convergence min/p50/p95/max ms | exhaustion min/p50/p95/max | attempts min/p50/p95/max | CAS count p50/p95 us | success count p50/p95 us | normalized load1 min/p50/p95/max |
|---:|---:|---:|---:|---:|---:|---:|
| 50 | 3866/3991/4371/6378 | 0.042/0.167/0.208/0.250 | 49/52/54/54 | 623 334745/436097 | 407 11664/314395 | 0.041/0.070/0.147/0.164 |
| 100 | 4126/4235/9864/12092 | 0.083/0.167/0.250/0.250 | 49/52/54/54 | 632 334811/436683 | 400 12091/319411 | 0.041/0.074/0.135/0.166 |
| 200 | 4208/4566/8168/11032 | 0.083/0.167/0.208/0.208 | 50/51/53/53 | 624 334815/436592 | 405 11738/319824 | 0.043/0.075/0.160/0.166 |

## Interpretation

- Every cell converged 20/20 with pending trace `[6,5,4,3,2,1,0]` and bounded worker fairness.
- The distributions remain experimental evidence only. Host load is observational and fail-open; it is not used to tune runtime policy.
- Keep production at 100 ms unless repeated evidence shows a robust improvement that survives interleaving and host-load variance.
