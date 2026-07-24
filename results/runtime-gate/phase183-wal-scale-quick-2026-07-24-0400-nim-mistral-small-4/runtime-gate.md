# Runtime provider gate campaign

- Name: `phase183-wal-scale-quick-nim-mistral-small-4`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-open`
- Selected route: `nim` / `nim-mistral-small-4`
- Provider success: `true`
- Provider latency: `700.930852ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 24
- Response SHA-256: `a0a42d07dc6585c4a548b1bb66e4943c8bb192740331bcaccc72dd5ad4325a78`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T07:11:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-24T07:10:36Z |
| model-binding:nim-mistral-small-4 | 0 | 1 | 1 | 119 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 119 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
