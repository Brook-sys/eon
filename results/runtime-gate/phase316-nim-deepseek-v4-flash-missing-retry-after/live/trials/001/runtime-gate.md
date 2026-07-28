# Runtime provider gate campaign

- Name: `phase316-nim-deepseek-v4-flash-missing-retry-after`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-deepseek-v4-flash-seeded`
- Selected route: `nim` / `nim-deepseek-v4-flash`
- Provider success: `false`
- Provider latency: `991.793943ms`
- Provider error class: `http`
- Provider error reason: ``
- Provider HTTP status: 503
- Provider Retry-After: `0s`
- Finish reason: ``
- Response bytes: 0
- Response SHA-256: ``
- Expected response configured: `false`
- Expected response exact match: `false`
- Structural comparison configured: `false`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 0/0/0
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:nim-deepseek-v4-flash | 0 | 1 | 1 | 96 | 1 | 2026-07-28T08:07:59Z |
| model-binding:nim-deepseek-v4-flash-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T08:07:58Z |
| model-provider:nim | 0 | 1 | 1 | 96 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
