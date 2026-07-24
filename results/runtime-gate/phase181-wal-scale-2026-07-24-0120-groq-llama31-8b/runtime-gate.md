# Runtime provider gate campaign

- Name: `phase181-wal-scale-groq-llama31-8b`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama31-8b`
- Selected route: `nim` / `nim-seeded-open`
- Provider success: `true`
- Provider latency: `684.145736ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 29
- Response SHA-256: `50e8f038286101c7581231a1c896e0d8571660c50f89cabf9efe8c5c10183084`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T04:30:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b | 0 | 0 | 0 | 0 | 1 | 2026-07-24T04:29:18Z |
| model-binding:nim-seeded-open | 0 | 1 | 1 | 117 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 117 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
