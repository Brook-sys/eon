# Runtime provider gate campaign

- Name: `phase198-wal-progress-groq-llama31-8b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded-open`
- Selected route: `groq` / `groq-llama31-8b`
- Provider success: `true`
- Provider latency: `312.293928ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 30
- Response SHA-256: `9526d454fd7bbabef06f34590393bd002515fea9945ce93f8dc958b47f8cd588`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T19:26:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b | 0 | 1 | 1 | 152 | 0 |  |
| model-binding:nim-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-24T19:25:05Z |
| model-provider:groq | 0 | 1 | 1 | 152 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
