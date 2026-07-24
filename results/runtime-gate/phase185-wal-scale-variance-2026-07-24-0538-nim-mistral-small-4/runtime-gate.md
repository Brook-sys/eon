# Runtime provider gate campaign

- Name: `phase185-wal-scale-variance-nim-mistral-small-4`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-open`
- Selected route: `nim` / `nim-mistral-small-4`
- Provider success: `true`
- Provider latency: `835.508833ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 27
- Response SHA-256: `9531d5e1135db08ec8bc26d55d77303d6513ade8ca51a5919c70eeca47c2a9dd`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T08:40:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-24T08:39:05Z |
| model-binding:nim-mistral-small-4 | 0 | 1 | 1 | 126 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 126 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
