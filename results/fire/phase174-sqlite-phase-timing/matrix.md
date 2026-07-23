# Phase 174 — SQLite phase timing matrix

Nine bounded campaigns, three rotating Latin blocks; production pacing unchanged.

| delay | outcome | n | elapsed p50/p95 us | callback p50/p95 us | write+CAS p50/p95 us | reload p50/p95 us | commit p50/p95 us |
|---:|---|---:|---:|---:|---:|---:|---:|
| 50 | success | 59 | 85151/451819 | 10/24 | 395/559 | 0/0 | 67271/147277 |
| 50 | cas_conflict | 97 | 335242/536175 | 11/25 | 329905/530319 | 1549/3418 | 0/0 |
| 100 | success | 61 | 16307/338315 | 9/22 | 381/510 | 0/0 | 8716/70568 |
| 100 | cas_conflict | 90 | 334936/436468 | 10/22 | 329854/430192 | 1647/3208 | 0/0 |
| 200 | success | 62 | 45568/482509 | 9/19 | 362/495 | 0/0 | 9230/219281 |
| 200 | cas_conflict | 92 | 335372/635257 | 11/33 | 330006/630333 | 1654/3087 | 0/0 |

Interpretation: `write_cas_us` includes SQLite lock acquisition, checkpoint write, and CAS predicate evaluation because database/sql does not expose those sub-phases separately. The matrix is diagnostic, not sufficient for production tuning.
