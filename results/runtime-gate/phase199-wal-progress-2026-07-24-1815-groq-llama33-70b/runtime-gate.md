# Runtime provider gate campaign

- Name: `phase199-wal-progress-groq-llama33-70b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded-open`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `382.318658ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 30
- Response SHA-256: `4924169df94593017e2214c3a3fe172329b57a2b87174690f351c63d321b5ebb`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T21:05:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 152 | 0 |  |
| model-binding:nim-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-24T21:04:32Z |
| model-provider:groq | 0 | 1 | 1 | 152 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
