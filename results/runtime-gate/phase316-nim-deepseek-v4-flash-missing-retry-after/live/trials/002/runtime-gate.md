# Runtime provider gate campaign

- Name: `phase316-nim-deepseek-v4-flash-missing-retry-after`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-deepseek-v4-flash-seeded`
- Selected route: `nim` / `nim-deepseek-v4-flash`
- Provider success: `true`
- Provider latency: `7.881349693s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 122
- Response SHA-256: `3678c4ff4a234b3aa2421db3bc342a234f50781561abcf11d5d7aac17b328473`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T08:08:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:nim-deepseek-v4-flash | 0 | 1 | 1 | 390 | 0 |  |
| model-binding:nim-deepseek-v4-flash-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T08:08:04Z |
| model-provider:nim | 0 | 1 | 1 | 390 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
